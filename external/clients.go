package external

import (
	customClient "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// k8s clientSet + custom clientSet

var (
	k8sClientSet    *kubernetes.Clientset
	customClientSet *customClient.Clientset
)

// InitClientSet 初始化集群中的clientSet
func InitClientSet() error {
	var (
		config *rest.Config
		err    error
	)

	// use in-cluster config
	config, err = rest.InClusterConfig()
	if err != nil {
		return err
	}

	// k8s client
	k8sClientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// custom client
	customClientSet, err = customClient.NewForConfig(config)
	if err != nil {
		return err
	}
	return nil
}

// GetK8sClientSet return k8s clientset
func GetK8sClientSet() *kubernetes.Clientset {
	return k8sClientSet
}

// GetCustomClientSet return custom clientset
func GetCustomClientSet() *customClient.Clientset {
	return customClientSet
}
