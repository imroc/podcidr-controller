# Auto Remove Taints Design

## Background

When deploying Flannel on TKE clusters that were originally created with VPC-CNI networking, newly added nodes get a taint `tke.cloud.tencent.com/eni-ip-unavailable:NoSchedule`. Normally, TKE's VPC-CNI components remove this taint when ready. However, when using Flannel instead of VPC-CNI, this taint is never removed, preventing pod scheduling.

## Goal

Add a feature to podcidr-controller that automatically removes specified taints from nodes.

## Design

### Behavior

- Taint removal operates independently from CIDR allocation
- Any node with matching taints will have them removed
- No node selector filtering - applies to all nodes
- Failed removals are retried using the existing workqueue mechanism

### Configuration Format

**CLI parameter:**
```bash
--remove-taints=tke.cloud.tencent.com/eni-ip-unavailable,node.kubernetes.io/not-ready:NoSchedule
```

**Supported formats:**
- `key` - Match all taints with this key, ignore value and effect
- `key:effect` - Match specified key and effect, ignore value
- `key=value:effect` - Exact match of key, value, and effect

**Helm values.yaml:**
```yaml
removeTaints:
  - tke.cloud.tencent.com/eni-ip-unavailable
  - node.kubernetes.io/not-ready:NoSchedule
```

### Code Structure

**New files:**
- `pkg/taint/taint.go` - Taint matching and removal logic
- `pkg/taint/taint_test.go` - Unit tests

**Modified files:**
- `cmd/root.go` - Add `--remove-taints` parameter parsing
- `pkg/controller/controller.go` - Call taint removal in `syncNode`
- `charts/podcidr-controller/values.yaml` - Add `removeTaints` config
- `charts/podcidr-controller/templates/deployment.yaml` - Pass parameter

### TaintRemover Implementation

```go
// pkg/taint/taint.go

// TaintRule represents a taint matching rule
type TaintRule struct {
    Key    string              // Required
    Value  *string             // Optional, nil matches any value
    Effect *corev1.TaintEffect // Optional, nil matches any effect
}

// TaintRemover handles removing matching taints from nodes
type TaintRemover struct {
    rules []TaintRule
}

// NewTaintRemover parses config string to create TaintRemover
// Input format: "key1,key2:NoSchedule,key3=value:NoExecute"
func NewTaintRemover(config string) (*TaintRemover, error)

// ShouldRemove checks if a taint matches any rule
func (r *TaintRemover) ShouldRemove(taint corev1.Taint) bool

// GetTaintsToRemove returns taints that should be removed from a node
func (r *TaintRemover) GetTaintsToRemove(node *corev1.Node) []corev1.Taint
```

### Controller Integration

```go
func (c *Controller) syncNode(ctx context.Context, key string) error {
    node, err := c.nodeLister.Get(key)
    if errors.IsNotFound(err) {
        return nil
    }
    if err != nil {
        return err
    }

    // 1. Handle taint removal (independent of CIDR allocation)
    if c.taintRemover != nil {
        if err := c.removeTaints(ctx, node); err != nil {
            return err // Will be requeued for retry
        }
    }

    // 2. Handle CIDR allocation (existing logic)
    // ...
}

func (c *Controller) removeTaints(ctx context.Context, node *corev1.Node) error {
    taintsToRemove := c.taintRemover.GetTaintsToRemove(node)
    if len(taintsToRemove) == 0 {
        return nil
    }

    nodeCopy := node.DeepCopy()
    nodeCopy.Spec.Taints = filterOutTaints(nodeCopy.Spec.Taints, taintsToRemove)

    _, err := c.clientset.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
    if err != nil {
        return fmt.Errorf("failed to remove taints from node %s: %w", node.Name, err)
    }

    klog.Infof("Removed taints %v from node %s", taintKeys(taintsToRemove), node.Name)
    return nil
}
```

### Helm Chart Changes

**values.yaml:**
```yaml
# Taints to automatically remove from nodes
# Supported formats: key, key:effect, key=value:effect
removeTaints: []
# Example:
# removeTaints:
#   - tke.cloud.tencent.com/eni-ip-unavailable
#   - node.kubernetes.io/not-ready:NoSchedule
```

**deployment.yaml:**
```yaml
args:
  - --cluster-cidr={{ .Values.clusterCIDR }}
  {{- if .Values.nodeSelector }}
  - --node-selector={{ .Values.nodeSelector }}
  {{- end }}
  {{- if .Values.removeTaints }}
  - --remove-taints={{ join "," .Values.removeTaints }}
  {{- end }}
```

## Testing

### Unit Tests

- Parsing: Verify parsing of all formats (key only, key:effect, key=value:effect)
- Matching: Verify wildcard logic (value wildcard, effect wildcard, exact match)
- Edge cases: Empty config, invalid format, empty node taints

### Integration Testing

Test cluster: `cls-6yd3a3g9` (TKE with Flannel and podcidr-controller deployed)

```bash
# Add test taint to a node
kubectl taint nodes <node-name> tke.cloud.tencent.com/eni-ip-unavailable:NoSchedule

# Verify controller removes the taint
kubectl logs -f deployment/podcidr-controller

# Confirm taint is removed
kubectl describe node <node-name> | grep -i taint
```
