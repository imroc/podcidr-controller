# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
make build          # Build binary to bin/podcidr-controller
make test           # Run all tests with verbose output
make lint           # Run golangci-lint
make docker-build-local  # Build Docker image locally
make clean          # Remove build artifacts
```

Run a single test:
```bash
go test -v -run TestAllocateNext ./pkg/cidr/...
```

## Architecture

This is a Kubernetes controller that automatically allocates Pod CIDRs to nodes. It replaces the built-in IPAM functionality of kube-controller-manager.

### Core Components

**pkg/cidr/allocator.go** - CIDR allocation engine using bitmap tracking
- Manages a boolean slice to track allocated subnets
- Thread-safe with mutex protection
- Key methods: `AllocateNext()`, `MarkAllocated()`, `Release()`
- Converts between CIDR strings and bitmap indices via IP arithmetic

**pkg/controller/controller.go** - Kubernetes controller using client-go informer pattern
- Watches Node resources via SharedInformerFactory
- Uses workqueue for rate-limited reconciliation
- On startup: scans existing nodes to rebuild allocation state
- On node add/update: allocates CIDR if `spec.podCIDR` is empty
- On node delete: releases CIDR back to pool

**cmd/root.go** - CLI entrypoint with leader election
- Uses cobra for CLI flags
- Supports HA via Lease-based leader election
- Required flag: `--cluster-cidr`

### Data Flow

1. Controller starts → syncs existing node CIDRs into allocator bitmap
2. Node created without podCIDR → queued to workqueue
3. Worker picks up node → `allocator.AllocateNext()` → updates `node.spec.podCIDR`
4. Node deleted → `allocator.Release()` returns CIDR to pool

### Deployment

Helm chart in `charts/podcidr-controller/` with:
- ClusterRole for node read/write and lease management
- Deployment with 2 replicas (leader election enabled)
- Tolerations for control-plane nodes
