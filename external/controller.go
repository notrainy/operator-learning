package external

import (
	"context"
	"fmt"
	"time"

	customapis "github.com/notrainy/operator-learning/pkg/apis/stable/v1beta1"
	"github.com/notrainy/operator-learning/pkg/consts"
	customClient "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned"
	customscheme "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned/scheme"
	customInformers "github.com/notrainy/operator-learning/pkg/generated/informers/externalversions/stable/v1beta1"
	customlisters "github.com/notrainy/operator-learning/pkg/generated/listers/stable/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	controllerAgentName = "redis-controller"
	rateLimitQueueName  = "custom-queue"
)

// Controller 自定义实现的Controller
type Controller struct {
	// k8sClientSet is a standard kubernetes clientset
	k8sClientSet kubernetes.Interface
	// customClientSet is a custom clientset
	customClientSet customClient.Interface

	deploymentLister appslisters.DeploymentLister
	// a function to determine if an informer has synced
	deploymentSynced cache.InformerSynced

	customResourceLister customlisters.RedisLister
	customResourceSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController 自定义的创建controller的函数
func NewController(
	ctx context.Context,
	k8sClientSet kubernetes.Interface,
	customClientSet customClient.Interface,
	k8sInformer appsinformers.DeploymentInformer,
	informer customInformers.RedisInformer) *Controller {

	logger := klog.FromContext(ctx)
	utilruntime.Must(customscheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("event broadcaster created...")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: k8sClientSet.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		k8sClientSet:         k8sClientSet,
		customClientSet:      customClientSet,
		deploymentLister:     k8sInformer.Lister(),
		deploymentSynced:     k8sInformer.Informer().HasSynced,
		customResourceLister: informer.Lister(),
		customResourceSynced: informer.Informer().HasSynced,
		workqueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), rateLimitQueueName),
		recorder:             recorder,
	}

	logger.Info("setting handlers...")

	// set up handlers when custom resource changes
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			controller.enqueueCRD(obj)
		},
		UpdateFunc: func(_, newObj interface{}) {
			controller.enqueueCRD(newObj)
		},
	})

	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Foo resource then the handler will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources.
	k8sInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(oldObj, newObj interface{}) {
			newDepl := newObj.(*appsv1.Deployment)
			oldDepl := oldObj.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}

			controller.handleObject(newObj)
		},
		DeleteFunc: controller.handleObject,
	})

	logger.Info("handlers setted")

	return controller
}

// Run controller执行
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentSynced, c.customResourceSynced); !ok {
		return fmt.Errorf("failed to wait for cache to sync")
	}

	logger.Info("starting workers")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("started workers")
	<-ctx.Done()
	logger.Info("shutdown workers")

	return nil
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// enqueueCRD takes a CRD resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than CRD.
func (c *Controller) enqueueCRD(obj interface{}) {
	var (
		key string
		err error
	)

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, quit := c.workqueue.Get()
	if quit {
		return false
	}
	logger := klog.FromContext(ctx)

	err := func(object interface{}) error {

		defer c.workqueue.Done(object)
		var (
			key string
			ok  bool
		)

		if key, ok = object.(string); !ok {
			c.workqueue.Forget(object)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %v", object))
			return nil
		}

		if err := c.syncHandler(ctx, key); err != nil {
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(object)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the CRD resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that CRD resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var (
		object metav1.Object
		ok     bool
	)
	logger := klog.FromContext(context.Background())

	if object, ok = obj.(metav1.Object); !ok {
		// if is deleted object, recover it
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			logger.V(4).Info("object is neither the metav1.object nor the deleted object")
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}

		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("recovered the deleted object")
	}

	logger.V(4).Info("processing the object", "object", klog.KObj(object))

	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		if ownerRef.Kind != consts.CRDKind {
			return
		}

		crd, err := c.customResourceLister.Redises(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("ignored the object", "object", klog.KObj(object))
			return
		}

		c.enqueueCRD(crd)
	}
}

// syncHandler 处理
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	// get namesapce and name from key
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %s", key))
		return nil
	}

	// get the CRD resource by namespace/name
	crd, err := c.customResourceLister.Redises(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("resource %s in work queue no longer exists", key))
			return nil
		}

		return err
	}

	// get deployment name of the crd
	deploymentName := crd.Spec.DeploymentName
	if deploymentName == "" {
		utilruntime.HandleError(fmt.Errorf("%s deployment name must be specified", key))
		return nil
	}

	// create/update the deployment for the CRD
	deployment, err := c.deploymentLister.Deployments(crd.Namespace).Get(deploymentName)
	if k8serrors.IsNotFound(err) {
		deployment, err = c.k8sClientSet.AppsV1().Deployments(crd.Namespace).Create(context.TODO(), newDeployment(crd), metav1.CreateOptions{})
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("create deployment %s %s failed", crd.Namespace, deploymentName))
		}
	}

	if err != nil {
		return err
	}

	if !metav1.IsControlledBy(deployment, crd) {
		return fmt.Errorf("deployment %s not controlled by the resource %s", deployment.Name, crd.Name)
	}

	if crd.Spec.Replicas != nil && *crd.Spec.Replicas != *deployment.Spec.Replicas {
		deployment, err = c.k8sClientSet.AppsV1().Deployments(crd.Namespace).Update(context.TODO(), newDeployment(crd), metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	c.recorder.Event(crd, corev1.EventTypeNormal, "Synced", "crd synced successfully")
	return nil
}

func newDeployment(r *customapis.Redis) *appsv1.Deployment {
	labels := map[string]string{
		"app": "redis",
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.Spec.DeploymentName,
			Namespace: r.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(r, customapis.SchemeGroupVersion.WithKind(consts.CRDKind)),
			},
		},

		Spec: appsv1.DeploymentSpec{
			Replicas: r.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: r.Spec.Image,
							Name:  "test",
						},
					},
				},
			},
		},
	}
	return deploy
}
