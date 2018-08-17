package profiles

import (
	"errors"

	pkgCluster "github.com/banzaicloud/pipeline/pkg/cluster"
	pkgProfileACSK "github.com/banzaicloud/pipeline/pkg/profiles/acsk"
	pkgProfileAKS "github.com/banzaicloud/pipeline/pkg/profiles/aks"
	"github.com/banzaicloud/pipeline/pkg/profiles/defaults"
	pkgProfileEC2 "github.com/banzaicloud/pipeline/pkg/profiles/ec2"
	pkgProfileEKS "github.com/banzaicloud/pipeline/pkg/profiles/eks"
	pkgProfileGKE "github.com/banzaicloud/pipeline/pkg/profiles/gke"
	pkgProfileOKE "github.com/banzaicloud/pipeline/pkg/profiles/oke"
)

type DefaultProfileManager interface {
	GetDefaultProfile() *pkgCluster.ClusterProfileResponse
	GetDefaultNodePoolName() string
	GetLocation() string
}

func GetDefaultProfileManager(distributionType string) (DefaultProfileManager, error) {

	manager := defaults.GetDefaultConfig()
	def, images := manager.GetConfig()

	switch distributionType {
	case pkgCluster.ACSK:
		return pkgProfileACSK.NewProfile(def.DefaultNodePoolName, &def.Distributions.ACSK), nil
	case pkgCluster.AKS:
		return pkgProfileAKS.NewProfile(def.DefaultNodePoolName, &def.Distributions.AKS), nil
	case pkgCluster.EC2:
		return pkgProfileEC2.NewProfile(def.DefaultNodePoolName, &def.Distributions.EC2, images.EC2.GetDefaultAmazonImage(def.Distributions.EC2.Location)), nil // todo refactor!!
	case pkgCluster.EKS:
		return pkgProfileEKS.NewProfile(def.DefaultNodePoolName, &def.Distributions.EKS, images.EKS.GetDefaultAmazonImage(def.Distributions.EKS.Location)), nil // todo refactor!!
	case pkgCluster.GKE:
		return pkgProfileGKE.NewProfile(def.DefaultNodePoolName, &def.Distributions.GKE), nil
	case pkgCluster.OKE:
		return pkgProfileOKE.NewProfile(def.DefaultNodePoolName, &def.Distributions.OKE), nil
	}

	return nil, errors.New("not supported distribution type")
}