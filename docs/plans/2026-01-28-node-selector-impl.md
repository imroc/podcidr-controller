# Node Selector Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add node selector functionality to filter which nodes receive PodCIDR allocation based on matchExpressions.

**Architecture:** New `pkg/selector` package handles label matching logic with 6 operators. Controller integrates selector check before CIDR allocation. CLI parses JSON flag and passes to controller.

**Tech Stack:** Go, client-go, Kubernetes corev1.Node, Cobra CLI

---

## Task 1: Create NodeSelector Package - Types and Parse

**Files:**
- Create: `pkg/selector/selector.go`
- Create: `pkg/selector/selector_test.go`

**Step 1: Write failing test for Parse function**

Create `pkg/selector/selector_test.go`:

```go
package selector

import (
	"testing"
)

func TestParseEmpty(t *testing.T) {
	s, err := Parse("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.MatchExpressions) != 0 {
		t.Errorf("expected empty expressions, got %d", len(s.MatchExpressions))
	}
}

func TestParseValidJSON(t *testing.T) {
	input := `[{"key":"node-type","operator":"In","values":["external"]}]`
	s, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.MatchExpressions) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(s.MatchExpressions))
	}
	if s.MatchExpressions[0].Key != "node-type" {
		t.Errorf("expected key 'node-type', got '%s'", s.MatchExpressions[0].Key)
	}
	if s.MatchExpressions[0].Operator != "In" {
		t.Errorf("expected operator 'In', got '%s'", s.MatchExpressions[0].Operator)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	_, err := Parse("not valid json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/selector/...`
Expected: FAIL - package not found

**Step 3: Write minimal implementation**

Create `pkg/selector/selector.go`:

```go
package selector

import (
	"encoding/json"
)

// Expression represents a single label selector requirement
type Expression struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// NodeSelector filters nodes based on label expressions
type NodeSelector struct {
	MatchExpressions []Expression
}

// Parse creates a NodeSelector from JSON string
// Empty string returns selector that matches all nodes
func Parse(jsonStr string) (*NodeSelector, error) {
	s := &NodeSelector{}
	if jsonStr == "" {
		return s, nil
	}

	var expressions []Expression
	if err := json.Unmarshal([]byte(jsonStr), &expressions); err != nil {
		return nil, err
	}
	s.MatchExpressions = expressions
	return s, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/selector/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/selector/
git commit -m "feat(selector): add NodeSelector types and Parse function"
```

---

## Task 2: Implement Matches Method - In/NotIn Operators

**Files:**
- Modify: `pkg/selector/selector.go`
- Modify: `pkg/selector/selector_test.go`

**Step 1: Write failing tests for In/NotIn operators**

Add to `pkg/selector/selector_test.go`:

```go
import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newNode(labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-node",
			Labels: labels,
		},
	}
}

func TestMatchesEmptySelector(t *testing.T) {
	s, _ := Parse("")
	node := newNode(map[string]string{"foo": "bar"})
	if !s.Matches(node) {
		t.Error("empty selector should match all nodes")
	}
}

func TestMatchesInOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"node-type","operator":"In","values":["external","edge"]}]`)

	// Should match
	node1 := newNode(map[string]string{"node-type": "external"})
	if !s.Matches(node1) {
		t.Error("expected node with 'external' to match")
	}

	// Should match
	node2 := newNode(map[string]string{"node-type": "edge"})
	if !s.Matches(node2) {
		t.Error("expected node with 'edge' to match")
	}

	// Should not match - different value
	node3 := newNode(map[string]string{"node-type": "internal"})
	if s.Matches(node3) {
		t.Error("expected node with 'internal' to not match")
	}

	// Should not match - label missing
	node4 := newNode(map[string]string{"other": "label"})
	if s.Matches(node4) {
		t.Error("expected node without label to not match")
	}
}

func TestMatchesNotInOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"zone","operator":"NotIn","values":["zone-a","zone-b"]}]`)

	// Should match - different value
	node1 := newNode(map[string]string{"zone": "zone-c"})
	if !s.Matches(node1) {
		t.Error("expected node with 'zone-c' to match NotIn")
	}

	// Should match - label missing
	node2 := newNode(map[string]string{"other": "label"})
	if !s.Matches(node2) {
		t.Error("expected node without zone label to match NotIn")
	}

	// Should not match
	node3 := newNode(map[string]string{"zone": "zone-a"})
	if s.Matches(node3) {
		t.Error("expected node with 'zone-a' to not match NotIn")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/selector/...`
Expected: FAIL - Matches method not found

**Step 3: Write implementation for Matches with In/NotIn**

Add to `pkg/selector/selector.go`:

```go
import (
	corev1 "k8s.io/api/core/v1"
)

// Matches returns true if the node matches all expressions (AND logic)
// Empty selector matches all nodes
func (s *NodeSelector) Matches(node *corev1.Node) bool {
	if len(s.MatchExpressions) == 0 {
		return true
	}

	for _, expr := range s.MatchExpressions {
		if !matchExpression(node.Labels, expr) {
			return false
		}
	}
	return true
}

func matchExpression(labels map[string]string, expr Expression) bool {
	value, hasLabel := labels[expr.Key]

	switch expr.Operator {
	case "In":
		if !hasLabel {
			return false
		}
		return containsString(expr.Values, value)

	case "NotIn":
		if !hasLabel {
			return true
		}
		return !containsString(expr.Values, value)

	default:
		return false
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/selector/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/selector/
git commit -m "feat(selector): implement Matches with In/NotIn operators"
```

---

## Task 3: Implement Exists/DoesNotExist Operators

**Files:**
- Modify: `pkg/selector/selector.go`
- Modify: `pkg/selector/selector_test.go`

**Step 1: Write failing tests**

Add to `pkg/selector/selector_test.go`:

```go
func TestMatchesExistsOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"node-type","operator":"Exists"}]`)

	// Should match - label exists
	node1 := newNode(map[string]string{"node-type": "anything"})
	if !s.Matches(node1) {
		t.Error("expected node with label to match Exists")
	}

	// Should not match - label missing
	node2 := newNode(map[string]string{"other": "label"})
	if s.Matches(node2) {
		t.Error("expected node without label to not match Exists")
	}
}

func TestMatchesDoesNotExistOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"vpc-cni","operator":"DoesNotExist"}]`)

	// Should match - label missing
	node1 := newNode(map[string]string{"other": "label"})
	if !s.Matches(node1) {
		t.Error("expected node without label to match DoesNotExist")
	}

	// Should not match - label exists
	node2 := newNode(map[string]string{"vpc-cni": "true"})
	if s.Matches(node2) {
		t.Error("expected node with label to not match DoesNotExist")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "Exists" ./pkg/selector/...`
Expected: FAIL

**Step 3: Add Exists/DoesNotExist cases**

Update `matchExpression` in `pkg/selector/selector.go`:

```go
func matchExpression(labels map[string]string, expr Expression) bool {
	value, hasLabel := labels[expr.Key]

	switch expr.Operator {
	case "In":
		if !hasLabel {
			return false
		}
		return containsString(expr.Values, value)

	case "NotIn":
		if !hasLabel {
			return true
		}
		return !containsString(expr.Values, value)

	case "Exists":
		return hasLabel

	case "DoesNotExist":
		return !hasLabel

	default:
		return false
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/selector/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/selector/
git commit -m "feat(selector): implement Exists/DoesNotExist operators"
```

---

## Task 4: Implement Gt/Lt Operators

**Files:**
- Modify: `pkg/selector/selector.go`
- Modify: `pkg/selector/selector_test.go`

**Step 1: Write failing tests**

Add to `pkg/selector/selector_test.go`:

```go
func TestMatchesGtOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"priority","operator":"Gt","values":["5"]}]`)

	// Should match - 10 > 5
	node1 := newNode(map[string]string{"priority": "10"})
	if !s.Matches(node1) {
		t.Error("expected node with priority=10 to match Gt 5")
	}

	// Should not match - 5 not > 5
	node2 := newNode(map[string]string{"priority": "5"})
	if s.Matches(node2) {
		t.Error("expected node with priority=5 to not match Gt 5")
	}

	// Should not match - 3 < 5
	node3 := newNode(map[string]string{"priority": "3"})
	if s.Matches(node3) {
		t.Error("expected node with priority=3 to not match Gt 5")
	}

	// Should not match - label missing
	node4 := newNode(map[string]string{})
	if s.Matches(node4) {
		t.Error("expected node without label to not match Gt")
	}

	// Should not match - non-numeric value
	node5 := newNode(map[string]string{"priority": "high"})
	if s.Matches(node5) {
		t.Error("expected node with non-numeric value to not match Gt")
	}
}

func TestMatchesLtOperator(t *testing.T) {
	s, _ := Parse(`[{"key":"priority","operator":"Lt","values":["5"]}]`)

	// Should match - 3 < 5
	node1 := newNode(map[string]string{"priority": "3"})
	if !s.Matches(node1) {
		t.Error("expected node with priority=3 to match Lt 5")
	}

	// Should not match - 5 not < 5
	node2 := newNode(map[string]string{"priority": "5"})
	if s.Matches(node2) {
		t.Error("expected node with priority=5 to not match Lt 5")
	}

	// Should not match - 10 > 5
	node3 := newNode(map[string]string{"priority": "10"})
	if s.Matches(node3) {
		t.Error("expected node with priority=10 to not match Lt 5")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "Gt|Lt" ./pkg/selector/...`
Expected: FAIL

**Step 3: Add Gt/Lt cases**

Update `matchExpression` in `pkg/selector/selector.go`:

```go
import (
	"strconv"
)

func matchExpression(labels map[string]string, expr Expression) bool {
	value, hasLabel := labels[expr.Key]

	switch expr.Operator {
	case "In":
		if !hasLabel {
			return false
		}
		return containsString(expr.Values, value)

	case "NotIn":
		if !hasLabel {
			return true
		}
		return !containsString(expr.Values, value)

	case "Exists":
		return hasLabel

	case "DoesNotExist":
		return !hasLabel

	case "Gt":
		if !hasLabel || len(expr.Values) == 0 {
			return false
		}
		labelVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		compareVal, err := strconv.ParseInt(expr.Values[0], 10, 64)
		if err != nil {
			return false
		}
		return labelVal > compareVal

	case "Lt":
		if !hasLabel || len(expr.Values) == 0 {
			return false
		}
		labelVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return false
		}
		compareVal, err := strconv.ParseInt(expr.Values[0], 10, 64)
		if err != nil {
			return false
		}
		return labelVal < compareVal

	default:
		return false
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/selector/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/selector/
git commit -m "feat(selector): implement Gt/Lt operators"
```

---

## Task 5: Test Multiple Expressions (AND Logic)

**Files:**
- Modify: `pkg/selector/selector_test.go`

**Step 1: Write test for AND logic**

Add to `pkg/selector/selector_test.go`:

```go
func TestMatchesMultipleExpressions(t *testing.T) {
	// node-type=edge AND zone!=zone-a
	s, _ := Parse(`[{"key":"node-type","operator":"In","values":["edge"]},{"key":"zone","operator":"NotIn","values":["zone-a"]}]`)

	// Should match - both conditions met
	node1 := newNode(map[string]string{"node-type": "edge", "zone": "zone-b"})
	if !s.Matches(node1) {
		t.Error("expected node matching both conditions to match")
	}

	// Should not match - first condition fails
	node2 := newNode(map[string]string{"node-type": "internal", "zone": "zone-b"})
	if s.Matches(node2) {
		t.Error("expected node failing first condition to not match")
	}

	// Should not match - second condition fails
	node3 := newNode(map[string]string{"node-type": "edge", "zone": "zone-a"})
	if s.Matches(node3) {
		t.Error("expected node failing second condition to not match")
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "Multiple" ./pkg/selector/...`
Expected: PASS (already implemented via loop in Matches)

**Step 3: Commit**

```bash
git add pkg/selector/
git commit -m "test(selector): add multiple expressions AND logic test"
```

---

## Task 6: Integrate Selector into CLI

**Files:**
- Modify: `cmd/root.go`

**Step 1: Add flag and parse**

Add to `cmd/root.go` imports and vars:

```go
import (
	"github.com/imroc/podcidr-controller/pkg/selector"
)

var (
	// ... existing vars ...
	nodeSelectorStr string
)
```

Add flag in `init()`:

```go
rootCmd.Flags().StringVar(&nodeSelectorStr, "node-selector", "", "JSON array of matchExpressions to filter nodes for CIDR allocation")
```

Update `runController` function:

```go
func runController(ctx context.Context, clientset kubernetes.Interface) error {
	nodeSelector, err := selector.Parse(nodeSelectorStr)
	if err != nil {
		return fmt.Errorf("failed to parse node-selector: %w", err)
	}

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute*10)

	ctrl, err := controller.NewController(clientset, informerFactory, clusterCIDR, nodeCIDRMaskSize, nodeSelector)
	if err != nil {
		return err
	}

	informerFactory.Start(ctx.Done())

	return ctrl.Run(ctx, 2)
}
```

**Step 2: Run build to verify syntax**

Run: `go build ./cmd/...`
Expected: FAIL - controller.NewController signature mismatch

**Step 3: Commit partial change**

```bash
git add cmd/root.go
git commit -m "feat(cli): add --node-selector flag parsing"
```

---

## Task 7: Integrate Selector into Controller

**Files:**
- Modify: `pkg/controller/controller.go`

**Step 1: Update Controller struct and constructor**

Update `pkg/controller/controller.go`:

```go
import (
	"github.com/imroc/podcidr-controller/pkg/selector"
)

type Controller struct {
	clientset    kubernetes.Interface
	nodeLister   corelister.NodeLister
	nodeSynced   cache.InformerSynced
	workqueue    workqueue.TypedRateLimitingInterface[string]
	allocator    *cidr.Allocator
	clusterCIDR  string
	nodeSelector *selector.NodeSelector
}

func NewController(
	clientset kubernetes.Interface,
	informerFactory informers.SharedInformerFactory,
	clusterCIDR string,
	nodeMaskSize int,
	nodeSelector *selector.NodeSelector,
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
```

**Step 2: Update syncNode to check selector**

Update `syncNode` in `pkg/controller/controller.go`:

```go
func (c *Controller) syncNode(ctx context.Context, key string) error {
	node, err := c.nodeLister.Get(key)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
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
		c.allocator.Release(cidrBlock)
		return fmt.Errorf("failed to update node %s with CIDR %s: %w", node.Name, cidrBlock, err)
	}

	klog.Infof("Allocated CIDR %s to node %s", cidrBlock, node.Name)
	return nil
}
```

**Step 3: Update syncExistingNodes to check selector**

Update `syncExistingNodes` in `pkg/controller/controller.go`:

```go
func (c *Controller) syncExistingNodes() error {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.Spec.PodCIDR != "" {
			// Only mark as allocated if node matches selector
			// This prevents reserving CIDRs for nodes we don't manage
			if !c.nodeSelector.Matches(node) {
				klog.V(4).Infof("Skipping existing CIDR %s for non-matching node %s", node.Spec.PodCIDR, node.Name)
				continue
			}
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
```

**Step 4: Run build to verify**

Run: `go build ./...`
Expected: PASS

**Step 5: Run tests**

Run: `go test -v ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/controller/controller.go
git commit -m "feat(controller): integrate NodeSelector for filtering nodes"
```

---

## Task 8: Update Helm Chart

**Files:**
- Modify: `charts/podcidr-controller/values.yaml`
- Modify: `charts/podcidr-controller/templates/deployment.yaml`

**Step 1: Add allocateNodeSelector to values.yaml**

Add to `charts/podcidr-controller/values.yaml` after `nodeCIDRMaskSize`:

```yaml
nodeCIDRMaskSize: 24

# Node selector for CIDR allocation (matchExpressions JSON)
# Only nodes matching this selector will receive PodCIDR allocation
# Empty means allocate to all nodes (default, backward compatible)
# Example: '[{"key":"node-type","operator":"In","values":["external"]}]'
allocateNodeSelector: ""
```

**Step 2: Update deployment.yaml to pass flag**

Update `charts/podcidr-controller/templates/deployment.yaml` args section:

```yaml
          args:
            - --cluster-cidr={{ .Values.clusterCIDR }}
            - --node-cidr-mask-size={{ .Values.nodeCIDRMaskSize }}
            {{- if .Values.allocateNodeSelector }}
            - --node-selector={{ .Values.allocateNodeSelector }}
            {{- end }}
            {{- if .Values.leaderElection.enabled }}
```

**Step 3: Commit**

```bash
git add charts/podcidr-controller/
git commit -m "feat(helm): add allocateNodeSelector configuration"
```

---

## Task 9: Update Documentation

**Files:**
- Modify: `README.md`
- Modify: `README_zh.md`

**Step 1: Update README.md**

Add to Configuration table in `README.md`:

```markdown
| `allocateNodeSelector`    | Node selector for CIDR allocation (JSON matchExpressions) | `""`                                 |
```

Add new section after "Usage Example":

```markdown
## Node Selector

By default, the controller allocates PodCIDRs to all nodes. You can use `--node-selector` to filter which nodes receive allocation.

### Only allocate to external nodes

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"node.kubernetes.io/instance-type","operator":"In","values":["external"]}]'
```

### Exclude VPC-CNI nodes

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"networking.cloud.tencent.com/vpc-cni","operator":"DoesNotExist"}]'
```

### Supported Operators

- `In` - Label value must be in the specified list
- `NotIn` - Label value must not be in the specified list
- `Exists` - Label must exist (value ignored)
- `DoesNotExist` - Label must not exist
- `Gt` - Label value (integer) must be greater than specified
- `Lt` - Label value (integer) must be less than specified

Multiple expressions use AND logic (all must match).
```

**Step 2: Update README_zh.md with Chinese translation**

Add corresponding Chinese content to `README_zh.md`.

**Step 3: Commit**

```bash
git add README.md README_zh.md
git commit -m "docs: add node selector documentation"
```

---

## Task 10: Final Verification

**Step 1: Run all tests**

Run: `make test`
Expected: PASS

**Step 2: Run linter**

Run: `make lint`
Expected: PASS

**Step 3: Build binary**

Run: `make build`
Expected: PASS

**Step 4: Verify help output**

Run: `./bin/podcidr-controller --help`
Expected: Shows `--node-selector` flag

**Step 5: Final commit if needed**

If any fixes were required, commit them with appropriate message.

---

**Plan complete and saved to `docs/plans/2026-01-28-node-selector-impl.md`. Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
