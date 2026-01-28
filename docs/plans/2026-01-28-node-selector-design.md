# Node Selector Design

## Overview

Add node selector functionality to podcidr-controller, allowing selective PodCIDR allocation based on node labels. This addresses hybrid environments like TKE where some nodes use VPC-CNI (no PodCIDR needed) while others (external/IDC nodes) require PodCIDR allocation.

## Requirements

- Filter nodes using matchExpressions syntax (similar to Kubernetes nodeAffinity)
- Support all 6 operators: `In`, `NotIn`, `Exists`, `DoesNotExist`, `Gt`, `Lt`
- Configure via command-line parameter `--node-selector`
- Backward compatible: no selector means allocate to all nodes
- Multiple expressions use AND logic
- Respond dynamically to node label changes

## Command Line Interface

New parameter:

```
--node-selector='[{"key":"node-type","operator":"In","values":["external","edge"]}]'
```

JSON format: array of expressions, each with:
- `key`: label key
- `operator`: one of `In`, `NotIn`, `Exists`, `DoesNotExist`, `Gt`, `Lt`
- `values`: array of values (optional for Exists/DoesNotExist)

Default behavior: when not configured, allocate to all nodes (backward compatible).

## Core Implementation

### New Package: `pkg/selector/selector.go`

```go
type NodeSelector struct {
    MatchExpressions []Expression
}

type Expression struct {
    Key      string   `json:"key"`
    Operator string   `json:"operator"`
    Values   []string `json:"values,omitempty"`
}

func (s *NodeSelector) Matches(node *corev1.Node) bool
```

`Matches` method iterates all expressions, returns true only if all match. Empty selector (not configured) always returns true.

### Changes to `cmd/root.go`

- Add `--node-selector` string flag
- Parse JSON into `[]Expression`
- Pass to Controller constructor

### Changes to `pkg/controller/controller.go`

- Add `nodeSelector *selector.NodeSelector` field to Controller struct
- `syncNode`: check `nodeSelector.Matches(node)` before allocation, skip if not matched
- `syncExistingNodes`: also check match to avoid marking unmanaged node CIDRs as allocated

## Data Flow

```
Node Event (Add/Update)
       ↓
   Enqueue to workqueue
       ↓
   syncNode processing
       ↓
┌──────────────────────┐
│ node.Spec.PodCIDR    │──── Has CIDR ────→ Skip
│ is empty?            │
└──────────────────────┘
       ↓ Empty
┌──────────────────────┐
│ nodeSelector.Matches │──── Not match ──→ Skip
│ node matches?        │
└──────────────────────┘
       ↓ Match
   AllocateNext() allocate CIDR
       ↓
   Update node.Spec.PodCIDR
```

### Dynamic Label Changes

Node label changes trigger Update events, re-enqueue for processing. If the node now matches selector and has no PodCIDR, it will be allocated automatically.

### CIDR Release on Node Deletion

`handleNodeDelete` keeps original logic: release CIDR if node has one, without checking selector (node may no longer match at deletion time).

## Usage Examples

### Scenario: TKE - Only allocate to external nodes

External nodes labeled with `node.kubernetes.io/instance-type=external`:

```bash
podcidr-controller \
  --cluster-cidr=10.244.0.0/16 \
  --node-selector='[{"key":"node.kubernetes.io/instance-type","operator":"In","values":["external"]}]'
```

### Scenario: Exclude VPC-CNI nodes

VPC-CNI nodes labeled with `networking.tke.cloud.tencent.com/vpc-cni=true`:

```bash
podcidr-controller \
  --cluster-cidr=10.244.0.0/16 \
  --node-selector='[{"key":"networking.tke.cloud.tencent.com/vpc-cni","operator":"DoesNotExist"}]'
```

### Scenario: Multiple conditions

Only allocate to nodes with `node-type=edge` AND not in `zone-a`:

```bash
podcidr-controller \
  --cluster-cidr=10.244.0.0/16 \
  --node-selector='[{"key":"node-type","operator":"In","values":["edge"]},{"key":"zone","operator":"NotIn","values":["zone-a"]}]'
```

## File Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/selector/selector.go` | Add | NodeSelector struct and Matches method |
| `pkg/selector/selector_test.go` | Add | Unit tests for all operators |
| `cmd/root.go` | Modify | Add `--node-selector` flag parsing |
| `pkg/controller/controller.go` | Modify | Integrate NodeSelector filtering |
| `pkg/controller/controller_test.go` | Modify | Add selector-related test cases |
| `charts/podcidr-controller/values.yaml` | Modify | Add nodeSelector config |
| `charts/podcidr-controller/templates/deployment.yaml` | Modify | Pass nodeSelector parameter |
| `README.md` | Modify | Add node selector documentation |
