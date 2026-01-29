package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/imroc/podcidr-controller/pkg/cidr"
	"github.com/imroc/podcidr-controller/pkg/selector"
	"github.com/imroc/podcidr-controller/pkg/taint"
)

type Controller struct {
	clientset     kubernetes.Interface
	nodeLister    corelister.NodeLister
	nodeSynced    cache.InformerSynced
	workqueue     workqueue.TypedRateLimitingInterface[string]
	allocator     *cidr.Allocator
	clusterCIDR   string
	nodeSelector  *selector.NodeSelector
	taintRemover  *taint.TaintRemover
}

func NewController(
	clientset kubernetes.Interface,
	informerFactory informers.SharedInformerFactory,
	clusterCIDR string,
	nodeMaskSize int,
	nodeSelector *selector.NodeSelector,
	taintRemover *taint.TaintRemover,
) (*Controller, error) {
	allocator, err := cidr.NewAllocator(clusterCIDR, nodeMaskSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create CIDR allocator: %w", err)
	}

	nodeInformer := informerFactory.Core().V1().Nodes()

	c := &Controller{
		clientset:    clientset,
		nodeLister:   nodeInformer.Lister(),
		nodeSynced:   nodeInformer.Informer().HasSynced,
		workqueue:    workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		allocator:    allocator,
		clusterCIDR:  clusterCIDR,
		nodeSelector: nodeSelector,
		taintRemover: taintRemover,
	}

	_, _ = nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: c.enqueueNode,
		UpdateFunc: func(old, new interface{}) {
			c.enqueueNode(new)
		},
		DeleteFunc: c.handleNodeDelete,
	})

	return c, nil
}

func (c *Controller) enqueueNode(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) handleNodeDelete(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			runtime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			runtime.HandleError(fmt.Errorf("tombstone contained object that is not a Node %#v", obj))
			return
		}
	}

	if node.Spec.PodCIDR != "" {
		if err := c.allocator.Release(node.Spec.PodCIDR); err != nil {
			klog.Warningf("Failed to release CIDR %s for deleted node %s: %v", node.Spec.PodCIDR, node.Name, err)
		} else {
			klog.Infof("Released CIDR %s from deleted node %s", node.Spec.PodCIDR, node.Name)
		}
	}
}

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting podcidr-controller")

	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(ctx.Done(), c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	if err := c.syncExistingNodes(); err != nil {
		return fmt.Errorf("failed to sync existing nodes: %w", err)
	}

	klog.Info("Starting workers")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	klog.Info("Started workers")
	<-ctx.Done()
	klog.Info("Shutting down workers")

	return nil
}

func (c *Controller) syncExistingNodes() error {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.Spec.PodCIDR != "" {
			// Reserve all existing CIDRs regardless of selector to prevent conflicts
			if err := c.allocator.MarkAllocated(node.Spec.PodCIDR); err != nil {
				klog.Warningf("Node %s has podCIDR %s which is not in cluster CIDR %s: %v",
					node.Name, node.Spec.PodCIDR, c.clusterCIDR, err)
			} else {
				klog.Infof("Marked existing CIDR %s as allocated for node %s", node.Spec.PodCIDR, node.Name)
			}
		}
	}

	return nil
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	key, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	defer c.workqueue.Done(key)

	err := c.syncNode(ctx, key)
	if err == nil {
		c.workqueue.Forget(key)
		return true
	}

	runtime.HandleError(fmt.Errorf("error syncing node %s: %v", key, err))
	c.workqueue.AddRateLimited(key)
	return true
}

func (c *Controller) syncNode(ctx context.Context, key string) error {
	node, err := c.nodeLister.Get(key)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// Handle taint removal (independent of CIDR allocation)
	if c.taintRemover != nil {
		if err := c.removeTaints(ctx, node); err != nil {
			return err
		}
	}

	// Already has CIDR
	if node.Spec.PodCIDR != "" {
		return nil
	}

	// Check if node matches selector
	if !c.nodeSelector.Matches(node) {
		klog.V(4).Infof("Node %s does not match selector, skipping", node.Name)
		return nil
	}

	cidrBlock, err := c.allocator.AllocateNext()
	if err != nil {
		return fmt.Errorf("failed to allocate CIDR for node %s: %w", node.Name, err)
	}

	nodeCopy := node.DeepCopy()
	nodeCopy.Spec.PodCIDR = cidrBlock
	nodeCopy.Spec.PodCIDRs = []string{cidrBlock}

	_, err = c.clientset.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		_ = c.allocator.Release(cidrBlock)
		return fmt.Errorf("failed to update node %s with CIDR %s: %w", node.Name, cidrBlock, err)
	}

	klog.Infof("Allocated CIDR %s to node %s", cidrBlock, node.Name)
	return nil
}

func (c *Controller) removeTaints(ctx context.Context, node *corev1.Node) error {
	taintsToRemove := c.taintRemover.GetTaintsToRemove(node)
	if len(taintsToRemove) == 0 {
		return nil
	}

	// Re-fetch node to get latest version
	freshNode, err := c.clientset.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", node.Name, err)
	}

	// Recalculate taints to remove based on fresh node
	taintsToRemove = c.taintRemover.GetTaintsToRemove(freshNode)
	if len(taintsToRemove) == 0 {
		return nil
	}

	nodeCopy := freshNode.DeepCopy()
	nodeCopy.Spec.Taints = taint.FilterOutTaints(nodeCopy.Spec.Taints, taintsToRemove)

	_, err = c.clientset.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove taints from node %s: %w", node.Name, err)
	}

	klog.Infof("Removed taints %v from node %s", taint.TaintKeys(taintsToRemove), node.Name)
	return nil
}
