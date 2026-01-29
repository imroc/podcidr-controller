[English](README.md) | 中文

# podcidr-controller

一个轻量级的 Kubernetes 节点 Pod CIDR 分配器。

## 概述

`podcidr-controller` 从配置的集群 CIDR 范围中自动为 Kubernetes 节点分配 Pod CIDR。它提供了与 `kube-controller-manager` 内置 IPAM 控制器类似的功能，但作为独立组件运行。

## 使用场景

在云厂商托管的 Kubernetes 集群上部署 Flannel 或 Calico（IPIP/VXLAN 模式）等 Overlay CNI 插件时，这些插件需要节点设置 `spec.podCIDR`。然而，某些云厂商在使用其原生 CNI 方案时会禁用 `kube-controller-manager` 中的节点 IPAM 控制器。

例如，腾讯云容器服务（TKE）在 VPC-CNI 网络模式下不会为节点分配 Pod CIDR，因为原生 CNI 不需要它们。如果你想在这类集群上部署 Flannel 或其他 Overlay CNI 插件，可以使用这个轻量级控制器来处理 Pod CIDR 分配。

## 功能特性

- 自动为节点分配 Pod CIDR
- 自动移除节点上指定的污点
- 基于位图追踪的顺序分配策略
- 支持 Leader 选举实现高可用
- 优雅处理已存在的节点 CIDR
- 节点删除时释放并复用 CIDR
- 多架构支持（amd64、arm64）

## 安装

### 使用 Helm

添加 Helm 仓库：

```bash
helm repo add podcidr-controller https://imroc.github.io/podcidr-controller
helm repo update
```

安装 Chart：

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set nodeCIDRMaskSize=24
```

### 配置参数

| 参数                      | 描述                                           | 默认值                               |
| ------------------------- | ---------------------------------------------- | ------------------------------------ |
| `clusterCIDR`             | Pod IP 的 CIDR 范围（必填）                    | `"10.244.0.0/16"`                    |
| `nodeCIDRMaskSize`        | 节点 CIDR 掩码大小                             | `24`                                 |
| `allocateNodeSelector`    | CIDR 分配的节点选择器（JSON matchExpressions） | `""`                                 |
| `removeTaints`            | 要自动移除的节点污点列表                       | `[]`                                 |
| `replicaCount`            | 副本数                                         | `2`                                  |
| `image.repository`        | 镜像仓库                                       | `docker.io/imroc/podcidr-controller` |
| `image.tag`               | 镜像标签                                       | `Chart.AppVersion`                   |
| `leaderElection.enabled`  | 启用 Leader 选举                               | `true`                               |
| `resources.limits.cpu`    | CPU 限制                                       | `100m`                               |
| `resources.limits.memory` | 内存限制                                       | `128Mi`                              |

## 使用示例

如果你的集群使用 `10.244.0.0/16` 作为 Pod 网络，每个节点使用 `/24` 子网：

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set nodeCIDRMaskSize=24
```

此配置支持：

- 256 个节点（2^(24-16) = 256 个子网）
- 每个节点 254 个 Pod（2^(32-24) - 2 = 254 个可用 IP）

## 节点选择器

默认情况下，控制器会为所有节点分配 PodCIDR。你可以使用 `--node-selector` 来筛选哪些节点需要分配。

### 仅为部分节点分配

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"node.kubernetes.io/instance-type","operator":"In","values":["external"]}]'
```

### 排除部分节点

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set allocateNodeSelector='[{"key":"test","operator":"DoesNotExist"}]'
```

### 支持的操作符

- `In` - 标签值必须在指定列表中
- `NotIn` - 标签值不能在指定列表中
- `Exists` - 标签必须存在（忽略值）
- `DoesNotExist` - 标签必须不存在
- `Gt` - 标签值（整数）必须大于指定值
- `Lt` - 标签值（整数）必须小于指定值

多个表达式使用 AND 逻辑（所有条件必须匹配）。

## 自动移除污点

控制器可以自动移除节点上指定的污点。这在托管 Kubernetes 集群上部署 Overlay CNI 插件时非常有用，因为节点可能存在阻止 Pod 调度的污点。

例如，TKE 集群在 VPC-CNI 模式下会给新节点添加 `tke.cloud.tencent.com/eni-ip-unavailable:NoSchedule` 污点。由于 Flannel 无法与原生 VPC-CNI 插件共存，需要先卸载 VPC-CNI 相关组件。卸载后，这个污点将不会被自动移除，导致 Pod 无法调度。

### 配置方式

```bash
helm install podcidr-controller podcidr-controller/podcidr-controller \
  --namespace kube-system \
  --set clusterCIDR=10.244.0.0/16 \
  --set removeTaints[0]=tke.cloud.tencent.com/eni-ip-unavailable
```

或在 values.yaml 中配置：

```yaml
removeTaints:
  - tke.cloud.tencent.com/eni-ip-unavailable
  - node.kubernetes.io/not-ready:NoSchedule
```

### 支持的格式

- `key` - 匹配所有该 key 的污点（任意 value、任意 effect）
- `key:effect` - 匹配指定 key 和 effect 的污点（任意 value）
- `key=value:effect` - 精确匹配 key、value 和 effect

### 示例

```bash
# 移除所有 key 为 "tke.cloud.tencent.com/eni-ip-unavailable" 的污点
--remove-taints=tke.cloud.tencent.com/eni-ip-unavailable

# 移除指定 effect 的污点
--remove-taints=node.kubernetes.io/not-ready:NoSchedule

# 精确匹配移除污点
--remove-taints=dedicated=gpu:NoSchedule

# 多个污点（逗号分隔）
--remove-taints=tke.cloud.tencent.com/eni-ip-unavailable,node.kubernetes.io/not-ready:NoSchedule
```

## 工作原理

1. 启动时，控制器扫描所有现有节点以构建分配位图
2. 已有 `spec.podCIDR` 的节点被标记为已分配（超出范围则跳过）
3. 没有 `spec.podCIDR` 的新节点将获得下一个可用的 CIDR
4. 当节点被删除时，其 CIDR 被释放以供复用

## 环境要求

- Kubernetes 1.29+
- Helm 3.0+

## 许可证

MIT License
