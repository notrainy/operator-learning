package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Redis is a specification for a Redis Resource
type Redis struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RedisSpec `json:"spec"`
}

// RedisSpec redis的相关参数
type RedisSpec struct {
	Image          string `json:"image"`
	Port           int    `json:"port"`
	TargetPort     int    `json:"targetPort"`
	Password       string `json:"password"`
	DeploymentName string `json:"deploymentName"`
	Replicas       *int32 `json:"replicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RedisList redis的List
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}
