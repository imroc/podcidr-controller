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

| 参数                      | 描述                      | 默认值                               |
| ------------------------- | ------------------------- | ------------------------------------ |
| `clusterCIDR`             | Pod IP 的 CIDR 范围（必填） | `"10.244.0.0/16"`                  |
| `nodeCIDRMaskSize`        | 节点 CIDR 掩码大小          | `24`                                 |
| `replicaCount`            | 副本数                     | `2`                                  |
| `image.repository`        | 镜像仓库                   | `docker.io/imroc/podcidr-controller` |
| `image.tag`               | 镜像标签                   | `Chart.AppVersion`                   |
| `leaderElection.enabled`  | 启用 Leader 选举           | `true`                               |
| `resources.limits.cpu`    | CPU 限制                   | `100m`                               |
| `resources.limits.memory` | 内存限制                   | `128Mi`                              |

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
