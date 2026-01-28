# Reserve CIDR for All Nodes Design

## Problem

When `allocateNodeSelector` is configured, the controller only reserves CIDRs for nodes matching the selector during startup. This causes CIDR conflicts when:

1. Old nodes (not matching selector) already have podCIDRs allocated by other components
2. New nodes (matching selector) get allocated the same CIDRs

Example scenario:
- Cloud VM nodes: use VPC-CNI, don't need podCIDR, don't match selector
- IDC nodes: need podCIDR for Flannel, match selector
- If a cloud VM node previously had a podCIDR assigned, new IDC nodes could receive the same CIDR

## Solution

Reserve CIDRs for all nodes with existing podCIDRs, regardless of whether they match `allocateNodeSelector`.

### Behavior Matrix

| Scenario | Matches allocateNodeSelector | Does NOT match allocateNodeSelector |
|----------|------------------------------|-------------------------------------|
| Startup with podCIDR | Reserve | Reserve |
| Startup without podCIDR | Allocate | Skip |
| Node deletion with podCIDR | Release | Release |

### Code Changes

Modify `syncExistingNodes()` in `pkg/controller/controller.go` to remove the selector check when reserving existing CIDRs.

Before:
```go
if !c.nodeSelector.Matches(node) {
    klog.V(4).Infof("Skipping existing CIDR %s for non-matching node %s", node.Spec.PodCIDR, node.Name)
    continue
}
```

After: Remove this check entirely - reserve all existing CIDRs.

No changes needed for:
- `handleNodeDelete()` - already releases CIDRs for all nodes
- `syncNode()` - selector check only affects allocation, which is correct
