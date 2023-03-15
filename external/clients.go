package external

import (
	"path"

	cfgloader "github.com/notrainy/operator-learning/config"
	customClient "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const defaultKubefile = "kube_dev.yaml"

// k8s clientSet + custom clientSet

var (
	k8sClientSet    *kubernetes.Clientset
	customClientSet *customClient.Clientset
)

// Init 初始化
func Init(inCluster bool) {
	if err := initClientSet(inCluster); err != nil {
		panic(err)
	}
}

// InitClientSet 初始化集群中的clientSet
func initClientSet(inCluster bool) error {
	var (
		config *rest.Config
		err    error
	)

	if inCluster {
		// use in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	} else {
		confPath := cfgloader.GetConfPath()
		config, err = clientcmd.BuildConfigFromFlags("", path.Join(confPath, defaultKubefile))
		if err != nil {
			return err
		}
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
