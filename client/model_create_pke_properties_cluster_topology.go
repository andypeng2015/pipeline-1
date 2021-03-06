/*
 * Pipeline API
 *
 * Pipeline v0.3.0 swagger
 *
 * API version: 0.3.0
 * Contact: info@banzaicloud.com
 */

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package client

type CreatePkePropertiesClusterTopology struct {
	Network    CreatePkePropertiesClusterTopologyNetwork    `json:"network,omitempty"`
	NodePools  []NodePoolsPke                               `json:"nodePools,omitempty"`
	Kubernetes CreatePkePropertiesClusterTopologyKubernetes `json:"kubernetes,omitempty"`
	Cri        CreatePkePropertiesClusterTopologyCri        `json:"cri,omitempty"`
}
