// Copyright © 2018 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package action

import (
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ess"
	"github.com/banzaicloud/pipeline/model"
	"github.com/banzaicloud/pipeline/pkg/cluster/acsk"
	pkgErrors "github.com/banzaicloud/pipeline/pkg/errors"
	"github.com/goph/emperror"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CreateACSKNodePoolAction describes the properties of an Alibaba cluster creation
type CreateACSKNodePoolAction struct {
	log       logrus.FieldLogger
	nodePools []*model.ACSKNodePoolModel
	context   *ACKContext
	region    string
}

// NewCreateACSKNodePoolAction creates a new CreateACSKNodePoolAction
func NewCreateACSKNodePoolAction(log logrus.FieldLogger, nodepools []*model.ACSKNodePoolModel, clusterContext *ACKContext, region string) *CreateACSKNodePoolAction {
	return &CreateACSKNodePoolAction{
		log:       log,
		nodePools: nodepools,
		context:   clusterContext,
		region:    region,
	}
}

// GetName returns the name of this CreateACSKNodePoolAction
func (a *CreateACSKNodePoolAction) GetName() string {
	return "CreateACSKNodePoolAction"
}

// ExecuteAction executes this CreateACSKNodePoolAction
func (a *CreateACSKNodePoolAction) ExecuteAction(input interface{}) (interface{}, error) {
	cluster, ok := input.(*acsk.AlibabaDescribeClusterResponse)
	if !ok {
		return nil, errors.New("invalid input")
	}

	if len(a.nodePools) == 0 {
		r, err := getClusterDetails(a.context.ClusterID, a.context.CSClient)
		if err != nil {
			return nil, emperror.With(err, "cluster", cluster.Name)
		}

		return r, nil
	}
	a.log.Infoln("EXECUTE CreateACSKNodePoolAction, cluster name", cluster.Name)

	errChan := make(chan error, len(a.nodePools))
	instanceIdsChan := make(chan []string, len(a.nodePools))
	defer close(errChan)
	defer close(instanceIdsChan)

	for _, nodePool := range a.nodePools {
		go func(nodePool *model.ACSKNodePoolModel) {
			scalingGroupRequest := ess.CreateCreateScalingGroupRequest()
			scalingGroupRequest.SetScheme(requests.HTTPS)
			scalingGroupRequest.SetDomain(fmt.Sprintf(acsk.AlibabaESSEndPointFmt, cluster.RegionID))
			scalingGroupRequest.SetContentType(requests.Json)

			a.log.WithFields(logrus.Fields{
				"region":        cluster.RegionID,
				"zone":          cluster.ZoneID,
				"instance_type": nodePool.InstanceType,
			}).Info("creating scaling group")

			scalingGroupRequest.MinSize = requests.NewInteger(nodePool.MinCount)
			scalingGroupRequest.MaxSize = requests.NewInteger(nodePool.MaxCount)
			scalingGroupRequest.VSwitchId = cluster.VSwitchID
			scalingGroupRequest.ScalingGroupName = fmt.Sprintf("asg-%s-%s", nodePool.Name, cluster.ClusterID)

			createScalingGroupResponse, err := a.context.ESSClient.CreateScalingGroup(scalingGroupRequest)
			if err != nil {
				errChan <- emperror.WrapWith(err, "could not create Scaling Group", "nodePoolName", nodePool.Name, "cluster", cluster.Name)
				instanceIdsChan <- nil
				return
			}

			nodePool.AsgID = createScalingGroupResponse.ScalingGroupId
			a.log.Infof("Scaling Group with id %s successfully created", nodePool.AsgID)
			a.log.Infof("Creating scaling configuration for group %s", nodePool.AsgID)

			scalingConfigurationRequest := ess.CreateCreateScalingConfigurationRequest()
			scalingConfigurationRequest.SetScheme(requests.HTTPS)
			scalingConfigurationRequest.SetDomain(fmt.Sprintf(acsk.AlibabaESSEndPointFmt, cluster.RegionID))
			scalingConfigurationRequest.SetContentType(requests.Json)

			scalingConfigurationRequest.ScalingGroupId = nodePool.AsgID
			scalingConfigurationRequest.SecurityGroupId = cluster.SecurityGroupID
			scalingConfigurationRequest.KeyPairName = cluster.Name
			scalingConfigurationRequest.InstanceType = nodePool.InstanceType
			scalingConfigurationRequest.SystemDiskCategory = "cloud_efficiency"
			scalingConfigurationRequest.ImageId = acsk.AlibabaDefaultImageId
			scalingConfigurationRequest.Tags =
				fmt.Sprintf(`{"pipeline-created":"true","pipeline-cluster":"%s","pipeline-nodepool":"%s"`,
					cluster.Name, nodePool.Name)

			createConfigurationResponse, err := a.context.ESSClient.CreateScalingConfiguration(scalingConfigurationRequest)
			if err != nil {
				errChan <- emperror.WrapWith(err, "could not create Scaling Configuration", "nodePoolName", nodePool.Name, "scalingGroupId", nodePool.AsgID, "cluster", cluster.Name)
				instanceIdsChan <- nil
				return
			}

			nodePool.ScalingConfigID = createConfigurationResponse.ScalingConfigurationId

			a.log.Infof("Scaling Configuration successfully created for group %s", nodePool.AsgID)

			enableSGRequest := ess.CreateEnableScalingGroupRequest()
			enableSGRequest.SetScheme(requests.HTTPS)
			enableSGRequest.SetDomain(fmt.Sprintf(acsk.AlibabaESSEndPointFmt, cluster.RegionID))
			enableSGRequest.SetContentType(requests.Json)

			enableSGRequest.ScalingGroupId = nodePool.AsgID
			enableSGRequest.ActiveScalingConfigurationId = nodePool.ScalingConfigID

			_, err = a.context.ESSClient.EnableScalingGroup(enableSGRequest)
			if err != nil {
				errChan <- emperror.WrapWith(err, "could not enable Scaling Group", "nodePoolName", nodePool.Name, "scalingGroupId", nodePool.AsgID, "cluster", cluster.Name)
				instanceIdsChan <- nil
				return
			}

			instanceIds, err := waitUntilScalingInstanceCreated(a.log, a.context.ESSClient, cluster.RegionID, nodePool)
			if err != nil {
				errChan <- emperror.With(err, "cluster", cluster.Name)
				instanceIdsChan <- nil
				return
			}
			// set running instance count for nodePool in DB
			nodePool.Count = len(instanceIds)

			errChan <- nil
			instanceIdsChan <- instanceIds
		}(nodePool)
	}

	caughtErrors := emperror.NewMultiErrorBuilder()

	var instanceIds []string
	var err error
	for i := 0; i < len(a.nodePools); i++ {
		err = <-errChan
		ids := <-instanceIdsChan
		if err != nil {
			caughtErrors.Add(err)
		} else {
			instanceIds = append(instanceIds, ids...)
		}
	}
	err = caughtErrors.ErrOrNil()
	if err != nil {
		return nil, pkgErrors.NewMultiErrorWithFormatter(err)
	}

	return attachInstancesToCluster(a.log, cluster.ClusterID, instanceIds, a.context.CSClient)
}

// UndoAction rolls back this CreateACSKNodePoolAction
func (a *CreateACSKNodePoolAction) UndoAction() (err error) {
	a.log.Info("EXECUTE UNDO CreateACSKNodePoolAction")
	return deleteNodepools(a.log, a.nodePools, a.context.ESSClient, a.region)
}
