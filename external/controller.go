package external

import (
	"context"
	"fmt"

	"github.com/notrainy/operator-learning/pkg/consts"
	customClient "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned"
	customscheme "github.com/notrainy/operator-learning/pkg/generated/clientset/versioned/scheme"
	customInformers "github.com/notrainy/operator-learning/pkg/generated/informers/externalversions/stable/v1beta1"
	customlisters "github.com/notrainy/operator-learning/pkg/generated/listers/stable/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
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

	return controller
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

// TODO
