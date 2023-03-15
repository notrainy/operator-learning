package main

import (
	"github.com/notrainy/operator-learning/config"
	"github.com/notrainy/operator-learning/external"
	"github.com/notrainy/operator-learning/tools"
	"k8s.io/klog/v2"
)

func main() {
	ctx := tools.SetupSignalHandler()

	// logs
	klog.InitFlags(nil)
	logger := klog.FromContext(ctx)

	config.InitConfig()

	// init the clientset by out-of-cluster kube config
	external.Init(false)
	// init the informers by clientset
	external.InitInformers()

	// new controller
	controller := external.NewController(ctx, external.GetK8sClientSet(), external.GetCustomClientSet(),
		external.GetKubeInformerFactory().Apps().V1().Deployments(),
		external.GetCustomInformerFactory().Stable().V1beta1().Redises())

	// start the informer
	external.GetKubeInformerFactory().Start(ctx.Done())
	external.GetCustomInformerFactory().Start(ctx.Done())

	if err := controller.Run(ctx, 2); err != nil {
		logger.Error(err, "error running controller")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
}
