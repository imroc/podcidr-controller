English | [中文](README_zh.md)

# podcidr-controller

A lightweight Pod CIDR allocator for Kubernetes nodes.

## Overview

`podcidr-controller` automatically allocates Pod CIDRs to Kubernetes nodes from a configured cluster CIDR range. It provides similar functionality to the built-in IPAM controller in `kube-controller-manager`, but runs as a standalone component.

## Use Cases

When deploying overlay CNI plugins like Flannel or Calico (IPIP/VXLAN mode) on managed Kubernetes clusters from cloud providers, these plugins require `spec.podCIDR` to be set on each node. However, some cloud providers disable the node IPAM controller in `kube-controller-manager` when using their native CNI solutions.

For example, Tencent Kubernetes Engine (TKE) clusters in VPC-CNI network mode do not allocate Pod CIDRs to nodes since the native CNI doesn't need them. If you want to deploy Flannel or other overlay CNI plugins on such clusters, you can use this lightweight controller to handle Pod CIDR allocation.

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

| Parameter                 | Description                                               | Default                              |
| ------------------------- | --------------------------------------------------------- | ------------------------------------ |
| `clusterCIDR`             | CIDR range for pod IPs (required)                         | `"10.244.0.0/16"`                    |
| `nodeCIDRMaskSize`        | Mask size for node CIDR                                   | `24`                                 |
| `allocateNodeSelector`    | Node selector for CIDR allocation (JSON matchExpressions) | `""`                                 |
| `replicaCount`            | Number of replicas                                        | `2`                                  |
| `image.repository`        | Image repository                                          | `docker.io/imroc/podcidr-controller` |
| `image.tag`               | Image tag                                                 | `Chart.AppVersion`                   |
| `leaderElection.enabled`  | Enable leader election                                    | `true`                               |
| `resources.limits.cpu`    | CPU limit                                                 | `100m`                               |
| `resources.limits.memory` | Memory limit                                              | `128Mi`                              |

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

## Node Selector

By default, the controller allocates PodCIDRs to all nodes. You can use `--node-selector` to filter which nodes receive allocation.

### Only allocate to some nodes

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"node.kubernetes.io/instance-type","operator":"In","values":["external"]}]'
```

### Exclude some nodes

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"test","operator":"DoesNotExist"}]'
```

### Supported Operators

- `In` - Label value must be in the specified list
- `NotIn` - Label value must not be in the specified list
- `Exists` - Label must exist (value ignored)
- `DoesNotExist` - Label must not exist
- `Gt` - Label value (integer) must be greater than specified
- `Lt` - Label value (integer) must be less than specified

Multiple expressions use AND logic (all must match).

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
