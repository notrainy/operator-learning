package external

import (
	"github.com/notrainy/operator-learning/pkg/consts"
	customeInformers "github.com/notrainy/operator-learning/pkg/generated/informers/externalversions"

	"k8s.io/client-go/informers"
)

var (
	kubeInformerFactory   informers.SharedInformerFactory
	customInformerFactory customeInformers.SharedInformerFactory
)

// InitInformers 初始化informers
func InitInformers() {
	kubeInformerFactory = informers.NewSharedInformerFactory(k8sClientSet, consts.InformerPeriod)
	customInformerFactory = customeInformers.NewSharedInformerFactory(customClientSet, consts.InformerPeriod)
}

// GetKubeInformerFactory return k8s informer factory
func GetKubeInformerFactory() informers.SharedInformerFactory {
	return kubeInformerFactory
}

// GetCustomInformerFactory return custom informer factory
func GetCustomInformerFactory() customeInformers.SharedInformerFactory {
	return customInformerFactory
}
