# PodCIDR Controller Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 实现一个轻量的 Kubernetes Pod CIDR 分配控制器，自动为节点分配 podCIDR。

**Architecture:** 使用 client-go 的 Informer 机制监听节点事件，通过位图算法管理 CIDR 分配，支持 Leader Election 实现高可用。

**Tech Stack:** Go 1.22+, client-go v0.32.x, cobra (CLI), Dockerfile 多架构构建, Helm Chart, GitHub Actions

---

## Task 1: 初始化 Go 项目

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `main.go`

**Step 1: 初始化 Go module**

Run: `go mod init github.com/imroc/podcidr-controller`

**Step 2: 创建 main.go 入口文件**

```go
package main

import (
	"fmt"
	"os"

	"github.com/imroc/podcidr-controller/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 3: Commit**

```bash
git add go.mod main.go
git commit -m "chore: init go module and main entry"
```

---

## Task 2: 实现 CLI 框架

**Files:**
- Create: `cmd/root.go`

**Step 1: 添加 cobra 依赖**

Run: `go get github.com/spf13/cobra`

**Step 2: 创建 root 命令**

```go
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	clusterCIDR      string
	nodeCIDRMaskSize int
)

var rootCmd = &cobra.Command{
	Use:   "podcidr-controller",
	Short: "A lightweight Pod CIDR allocator for Kubernetes nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

func init() {
	rootCmd.Flags().StringVar(&clusterCIDR, "cluster-cidr", "", "CIDR range for pod IPs (required)")
	rootCmd.Flags().IntVar(&nodeCIDRMaskSize, "node-cidr-mask-size", 24, "Mask size for node CIDR")
	rootCmd.MarkFlagRequired("cluster-cidr")
}

func Execute() error {
	return rootCmd.Execute()
}

func run() error {
	// TODO: implement controller logic
	return nil
}
```

**Step 3: 验证编译**

Run: `go build -o podcidr-controller .`
Expected: 编译成功，生成二进制文件

**Step 4: Commit**

```bash
git add cmd/root.go go.mod go.sum
git commit -m "feat: add CLI framework with cobra"
```

---

## Task 3: 实现 CIDR 分配器核心逻辑

**Files:**
- Create: `pkg/cidr/allocator.go`
- Create: `pkg/cidr/allocator_test.go`

**Step 1: 编写测试文件**

```go
package cidr

import (
	"testing"
)

func TestNewAllocator(t *testing.T) {
	alloc, err := NewAllocator("10.244.0.0/16", 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alloc.Total() != 256 {
		t.Errorf("expected 256 subnets, got %d", alloc.Total())
	}
}

func TestAllocateNext(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	cidr1, err := alloc.AllocateNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr1 != "10.244.0.0/24" {
		t.Errorf("expected 10.244.0.0/24, got %s", cidr1)
	}

	cidr2, err := alloc.AllocateNext()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr2 != "10.244.1.0/24" {
		t.Errorf("expected 10.244.1.0/24, got %s", cidr2)
	}
}

func TestMarkAllocated(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	err := alloc.MarkAllocated("10.244.5.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next allocation should skip index 5
	for i := 0; i < 5; i++ {
		alloc.AllocateNext()
	}
	cidr, _ := alloc.AllocateNext()
	if cidr != "10.244.6.0/24" {
		t.Errorf("expected 10.244.6.0/24, got %s", cidr)
	}
}

func TestRelease(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/24", 26)

	cidr1, _ := alloc.AllocateNext()
	alloc.Release(cidr1)

	cidr2, _ := alloc.AllocateNext()
	if cidr1 != cidr2 {
		t.Errorf("expected released CIDR to be reused")
	}
}

func TestAllocatorExhausted(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/24", 26) // Only 4 subnets

	for i := 0; i < 4; i++ {
		_, err := alloc.AllocateNext()
		if err != nil {
			t.Fatalf("unexpected error on allocation %d: %v", i, err)
		}
	}

	_, err := alloc.AllocateNext()
	if err == nil {
		t.Error("expected error when CIDR exhausted")
	}
}

func TestMarkAllocatedOutOfRange(t *testing.T) {
	alloc, _ := NewAllocator("10.244.0.0/16", 24)

	err := alloc.MarkAllocated("192.168.0.0/24")
	if err == nil {
		t.Error("expected error for out-of-range CIDR")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test ./pkg/cidr/... -v`
Expected: FAIL (allocator.go 不存在)

**Step 3: 实现 allocator.go**

```go
package cidr

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	ErrCIDRExhausted  = errors.New("CIDR range exhausted")
	ErrCIDROutOfRange = errors.New("CIDR out of cluster range")
	ErrInvalidCIDR    = errors.New("invalid CIDR format")
)

type Allocator struct {
	mu            sync.Mutex
	clusterCIDR   *net.IPNet
	maskSize      int
	total         int
	allocated     []bool
	nextCandidate int
}

func NewAllocator(clusterCIDR string, nodeMaskSize int) (*Allocator, error) {
	_, ipnet, err := net.ParseCIDR(clusterCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster CIDR: %w", err)
	}

	clusterMaskSize, _ := ipnet.Mask.Size()
	if nodeMaskSize <= clusterMaskSize {
		return nil, fmt.Errorf("node mask size (%d) must be larger than cluster mask size (%d)", nodeMaskSize, clusterMaskSize)
	}

	total := 1 << (nodeMaskSize - clusterMaskSize)

	return &Allocator{
		clusterCIDR:   ipnet,
		maskSize:      nodeMaskSize,
		total:         total,
		allocated:     make([]bool, total),
		nextCandidate: 0,
	}, nil
}

func (a *Allocator) Total() int {
	return a.total
}

func (a *Allocator) AllocateNext() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := 0; i < a.total; i++ {
		idx := (a.nextCandidate + i) % a.total
		if !a.allocated[idx] {
			a.allocated[idx] = true
			a.nextCandidate = (idx + 1) % a.total
			return a.indexToCIDR(idx), nil
		}
	}
	return "", ErrCIDRExhausted
}

func (a *Allocator) MarkAllocated(cidr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return err
	}
	a.allocated[idx] = true
	return nil
}

func (a *Allocator) Release(cidr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return err
	}
	a.allocated[idx] = false
	return nil
}

func (a *Allocator) IsAllocated(cidr string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx, err := a.cidrToIndex(cidr)
	if err != nil {
		return false
	}
	return a.allocated[idx]
}

func (a *Allocator) indexToCIDR(idx int) string {
	ip := make(net.IP, len(a.clusterCIDR.IP))
	copy(ip, a.clusterCIDR.IP)

	clusterMaskSize, _ := a.clusterCIDR.Mask.Size()
	bitsToShift := 32 - a.maskSize
	offset := idx << bitsToShift

	ipInt := ipToUint32(ip)
	ipInt += uint32(offset)
	resultIP := uint32ToIP(ipInt)

	return fmt.Sprintf("%s/%d", resultIP.String(), a.maskSize)
}

func (a *Allocator) cidrToIndex(cidr string) (int, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, ErrInvalidCIDR
	}

	maskSize, _ := ipnet.Mask.Size()
	if maskSize != a.maskSize {
		return 0, ErrCIDROutOfRange
	}

	if !a.clusterCIDR.Contains(ip) {
		return 0, ErrCIDROutOfRange
	}

	clusterIP := ipToUint32(a.clusterCIDR.IP)
	nodeIP := ipToUint32(ip.To4())
	bitsToShift := 32 - a.maskSize
	idx := int((nodeIP - clusterIP) >> bitsToShift)

	if idx < 0 || idx >= a.total {
		return 0, ErrCIDROutOfRange
	}

	return idx, nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIP(n uint32) net.IP {
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}
```

**Step 4: 运行测试验证通过**

Run: `go test ./pkg/cidr/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/cidr/
git commit -m "feat: implement CIDR allocator with bitmap"
```

---

## Task 4: 实现节点控制器

**Files:**
- Create: `pkg/controller/controller.go`

**Step 1: 添加 client-go 依赖**

Run: `go get k8s.io/client-go@v0.32.0 k8s.io/api@v0.32.0 k8s.io/apimachinery@v0.32.0`

**Step 2: 实现控制器**

```go
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/imroc/podcidr-controller/pkg/cidr"
)

type Controller struct {
	clientset   kubernetes.Interface
	nodeLister  corelister.NodeLister
	nodeSynced  cache.InformerSynced
	workqueue   workqueue.TypedRateLimitingInterface[string]
	allocator   *cidr.Allocator
	clusterCIDR string
}

func NewController(
	clientset kubernetes.Interface,
	informerFactory informers.SharedInformerFactory,
	clusterCIDR string,
	nodeMaskSize int,
) (*Controller, error) {
	allocator, err := cidr.NewAllocator(clusterCIDR, nodeMaskSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create CIDR allocator: %w", err)
	}

	nodeInformer := informerFactory.Core().V1().Nodes()

	c := &Controller{
		clientset:   clientset,
		nodeLister:  nodeInformer.Lister(),
		nodeSynced:  nodeInformer.Informer().HasSynced,
		workqueue:   workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		allocator:   allocator,
		clusterCIDR: clusterCIDR,
	}

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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
	nodes, err := c.nodeLister.List(nil)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.Spec.PodCIDR != "" {
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

	if node.Spec.PodCIDR != "" {
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
		c.allocator.Release(cidrBlock)
		return fmt.Errorf("failed to update node %s with CIDR %s: %w", node.Name, cidrBlock, err)
	}

	klog.Infof("Allocated CIDR %s to node %s", cidrBlock, node.Name)
	return nil
}
```

**Step 3: 验证编译**

Run: `go build ./...`
Expected: 编译成功

**Step 4: Commit**

```bash
git add pkg/controller/
git commit -m "feat: implement node controller with informer"
```

---

## Task 5: 实现 Leader Election

**Files:**
- Modify: `cmd/root.go`

**Step 1: 添加 leader election 依赖**

Run: `go get k8s.io/client-go/tools/leaderelection`

**Step 2: 更新 root.go 添加 leader election**

```go
package cmd

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/imroc/podcidr-controller/pkg/controller"
)

var (
	clusterCIDR      string
	nodeCIDRMaskSize int
	leaderElect      bool
	leaseDuration    time.Duration
	renewDeadline    time.Duration
	retryPeriod      time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "podcidr-controller",
	Short: "A lightweight Pod CIDR allocator for Kubernetes nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

func init() {
	rootCmd.Flags().StringVar(&clusterCIDR, "cluster-cidr", "", "CIDR range for pod IPs (required)")
	rootCmd.Flags().IntVar(&nodeCIDRMaskSize, "node-cidr-mask-size", 24, "Mask size for node CIDR")
	rootCmd.Flags().BoolVar(&leaderElect, "leader-elect", true, "Enable leader election for HA")
	rootCmd.Flags().DurationVar(&leaseDuration, "leader-elect-lease-duration", 15*time.Second, "Lease duration for leader election")
	rootCmd.Flags().DurationVar(&renewDeadline, "leader-elect-renew-deadline", 10*time.Second, "Renew deadline for leader election")
	rootCmd.Flags().DurationVar(&retryPeriod, "leader-elect-retry-period", 2*time.Second, "Retry period for leader election")
	rootCmd.MarkFlagRequired("cluster-cidr")
}

func Execute() error {
	return rootCmd.Execute()
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	if leaderElect {
		return runWithLeaderElection(ctx, clientset)
	}
	return runController(ctx, clientset)
}

func runWithLeaderElection(ctx context.Context, clientset kubernetes.Interface) error {
	id, err := os.Hostname()
	if err != nil {
		return err
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "kube-system"
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "podcidr-controller",
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				if err := runController(ctx, clientset); err != nil {
					klog.Fatalf("Controller error: %v", err)
				}
			},
			OnStoppedLeading: func() {
				klog.Info("Lost leadership")
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					return
				}
				klog.Infof("New leader elected: %s", identity)
			},
		},
	})

	return nil
}

func runController(ctx context.Context, clientset kubernetes.Interface) error {
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute*10)

	ctrl, err := controller.NewController(clientset, informerFactory, clusterCIDR, nodeCIDRMaskSize)
	if err != nil {
		return err
	}

	informerFactory.Start(ctx.Done())

	return ctrl.Run(ctx, 2)
}
```

**Step 3: 验证编译**

Run: `go build -o podcidr-controller .`
Expected: 编译成功

**Step 4: Commit**

```bash
git add cmd/root.go go.mod go.sum
git commit -m "feat: add leader election support"
```

---

## Task 6: 创建 Dockerfile

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Step 1: 创建 .dockerignore**

```
.git
*.md
docs/
charts/
```

**Step 2: 创建多阶段 Dockerfile**

```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o podcidr-controller .

FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /app/podcidr-controller .

USER 65532:65532

ENTRYPOINT ["/podcidr-controller"]
```

**Step 3: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat: add multi-arch Dockerfile"
```

---

## Task 7: 创建 Makefile

**Files:**
- Create: `Makefile`

**Step 1: 创建 Makefile**

```makefile
IMAGE ?= docker.io/imroc/podcidr-controller
TAG ?= latest
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: build
build:
	go build -o bin/podcidr-controller .

.PHONY: test
test:
	go test ./... -v

.PHONY: docker-build
docker-build:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) --push .

.PHONY: docker-build-local
docker-build-local:
	docker build -t $(IMAGE):$(TAG) .

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf bin/
```

**Step 2: 验证构建**

Run: `make build`
Expected: 编译成功，生成 bin/podcidr-controller

**Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add Makefile for build automation"
```

---

## Task 8: 创建 Helm Chart

**Files:**
- Create: `charts/podcidr-controller/Chart.yaml`
- Create: `charts/podcidr-controller/values.yaml`
- Create: `charts/podcidr-controller/templates/_helpers.tpl`
- Create: `charts/podcidr-controller/templates/deployment.yaml`
- Create: `charts/podcidr-controller/templates/serviceaccount.yaml`
- Create: `charts/podcidr-controller/templates/clusterrole.yaml`
- Create: `charts/podcidr-controller/templates/clusterrolebinding.yaml`

**Step 1: 创建 Chart.yaml**

```yaml
apiVersion: v2
name: podcidr-controller
description: A lightweight Pod CIDR allocator for Kubernetes nodes
type: application
version: 0.1.0
appVersion: "0.1.0"
home: https://github.com/imroc/podcidr-controller
sources:
  - https://github.com/imroc/podcidr-controller
maintainers:
  - name: roc
    url: https://github.com/imroc
```

**Step 2: 创建 values.yaml**

```yaml
replicaCount: 2

image:
  repository: docker.io/imroc/podcidr-controller
  tag: ""
  pullPolicy: IfNotPresent

clusterCIDR: ""
nodeCIDRMaskSize: 24

leaderElection:
  enabled: true
  leaseDuration: 15s
  renewDeadline: 10s
  retryPeriod: 2s

resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 50m
    memory: 64Mi

nodeSelector: {}
tolerations:
  - key: node-role.kubernetes.io/control-plane
    operator: Exists
    effect: NoSchedule
affinity: {}

serviceAccount:
  create: true
  name: ""
```

**Step 3: 创建 _helpers.tpl**

```yaml
{{/*
Expand the name of the chart.
*/}}
{{- define "podcidr-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "podcidr-controller.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "podcidr-controller.labels" -}}
helm.sh/chart: {{ include "podcidr-controller.chart" . }}
{{ include "podcidr-controller.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "podcidr-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "podcidr-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "podcidr-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "podcidr-controller.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "podcidr-controller.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
```

**Step 4: 创建 serviceaccount.yaml**

```yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "podcidr-controller.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "podcidr-controller.labels" . | nindent 4 }}
{{- end }}
```

**Step 5: 创建 clusterrole.yaml**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "podcidr-controller.fullname" . }}
  labels:
    {{- include "podcidr-controller.labels" . | nindent 4 }}
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
```

**Step 6: 创建 clusterrolebinding.yaml**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "podcidr-controller.fullname" . }}
  labels:
    {{- include "podcidr-controller.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "podcidr-controller.fullname" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "podcidr-controller.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
```

**Step 7: 创建 deployment.yaml**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "podcidr-controller.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "podcidr-controller.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "podcidr-controller.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "podcidr-controller.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "podcidr-controller.serviceAccountName" . }}
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          args:
            - --cluster-cidr={{ .Values.clusterCIDR }}
            - --node-cidr-mask-size={{ .Values.nodeCIDRMaskSize }}
            {{- if .Values.leaderElection.enabled }}
            - --leader-elect=true
            - --leader-elect-lease-duration={{ .Values.leaderElection.leaseDuration }}
            - --leader-elect-renew-deadline={{ .Values.leaderElection.renewDeadline }}
            - --leader-elect-retry-period={{ .Values.leaderElection.retryPeriod }}
            {{- else }}
            - --leader-elect=false
            {{- end }}
          env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

**Step 8: 验证 Helm Chart**

Run: `helm lint charts/podcidr-controller`
Expected: 0 chart(s) linted, 0 chart(s) failed

**Step 9: Commit**

```bash
git add charts/
git commit -m "feat: add Helm chart for deployment"
```

---

## Task 9: 创建 GitHub Actions 工作流

**Files:**
- Create: `.github/workflows/release.yaml`
- Create: `.github/workflows/ci.yaml`

**Step 1: 创建 CI 工作流**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run tests
        run: go test ./... -v

      - name: Build
        run: go build -o podcidr-controller .
```

**Step 2: 创建 Release 工作流**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: docker.io/imroc/podcidr-controller
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

  helm:
    runs-on: ubuntu-latest
    needs: docker
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Configure Git
        run: |
          git config user.name "$GITHUB_ACTOR"
          git config user.email "$GITHUB_ACTOR@users.noreply.github.com"

      - name: Install Helm
        uses: azure/setup-helm@v4

      - name: Update Chart version
        run: |
          VERSION=${GITHUB_REF#refs/tags/v}
          sed -i "s/^version:.*/version: $VERSION/" charts/podcidr-controller/Chart.yaml
          sed -i "s/^appVersion:.*/appVersion: \"$VERSION\"/" charts/podcidr-controller/Chart.yaml

      - name: Package Helm chart
        run: |
          helm package charts/podcidr-controller -d .helm-charts

      - name: Checkout gh-pages
        uses: actions/checkout@v4
        with:
          ref: gh-pages
          path: gh-pages

      - name: Update Helm repo
        run: |
          cp .helm-charts/*.tgz gh-pages/
          helm repo index gh-pages --url https://imroc.github.io/podcidr-controller

      - name: Push to gh-pages
        run: |
          cd gh-pages
          git add .
          git commit -m "Release Helm chart ${GITHUB_REF#refs/tags/}"
          git push
```

**Step 3: Commit**

```bash
git add .github/
git commit -m "feat: add GitHub Actions for CI and release"
```

---

## Task 10: 创建 README 文档

**Files:**
- Create: `README.md`

**Step 1: 创建 README.md**

```markdown
# podcidr-controller

A lightweight Pod CIDR allocator for Kubernetes nodes.

## Overview

`podcidr-controller` automatically allocates Pod CIDRs to Kubernetes nodes from a configured cluster CIDR range. It provides similar functionality to the built-in IPAM controller in `kube-controller-manager`, but runs as a standalone component.

## Features

- Automatic Pod CIDR allocation for nodes
- Sequential allocation strategy with bitmap tracking
- Leader election for high availability
- Graceful handling of existing node CIDRs
- CIDR release and reuse on node deletion
- Multi-architecture support (amd64, arm64)

## Installation

### Using Helm

Add the Helm repository:

```bash
helm repo add podcidr-controller https://imroc.github.io/podcidr-controller
helm repo update
```

Install the chart:

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set nodeCIDRMaskSize=24
```

### Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `clusterCIDR` | CIDR range for pod IPs (required) | `""` |
| `nodeCIDRMaskSize` | Mask size for node CIDR | `24` |
| `replicaCount` | Number of replicas | `2` |
| `image.repository` | Image repository | `docker.io/imroc/podcidr-controller` |
| `image.tag` | Image tag | `Chart.AppVersion` |
| `leaderElection.enabled` | Enable leader election | `true` |
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |

## Usage Example

If your cluster uses `10.244.0.0/16` for pod networking with `/24` subnets per node:

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set nodeCIDRMaskSize=24
```

This configuration allows:
- 256 nodes (2^(24-16) = 256 subnets)
- 254 pods per node (2^(32-24) - 2 = 254 usable IPs)

## How It Works

1. On startup, the controller scans all existing nodes to build an allocation bitmap
2. Nodes with existing `spec.podCIDR` are marked as allocated (skipped if out of range)
3. New nodes without `spec.podCIDR` receive the next available CIDR
4. When a node is deleted, its CIDR is released for reuse

## Requirements

- Kubernetes 1.29+
- Helm 3.0+

## License

MIT License
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add comprehensive README"
```

---

## Task 11: 初始化 gh-pages 分支

**Files:**
- Create: `index.yaml` (on gh-pages branch)

**Step 1: 创建并推送 gh-pages 分支**

```bash
git checkout --orphan gh-pages
git rm -rf .
echo "# Helm Chart Repository" > README.md
helm repo index . --url https://imroc.github.io/podcidr-controller
git add .
git commit -m "Initialize Helm chart repository"
git push origin gh-pages
git checkout main
```

---

## Task 12: 最终验证

**Step 1: 运行所有测试**

Run: `make test`
Expected: All tests pass

**Step 2: 验证 Helm chart**

Run: `helm lint charts/podcidr-controller`
Expected: No errors

**Step 3: 本地构建镜像测试**

Run: `make docker-build-local`
Expected: Image built successfully
```

