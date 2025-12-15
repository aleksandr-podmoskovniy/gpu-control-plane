<!--
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

---

title: "GPU Control Plane"
menuTitle: "GPU Control Plane"
moduleStatus: Experimental
weight: 10

---

# GPU Control Plane module

The GPU Control Plane module equips Deckhouse with a production-grade GPU
inventory and bootstrap layer. Controllers continuously discover NVIDIA GPUs,
keep their hardware profiles in custom resources, surface health metrics, and
prepare nodes for higher-level scheduling and resource governance.

## Description

The module focuses on the **inventory and diagnostics** stage of the GPU
control plane. It tracks Node and NodeFeature updates published by
node-feature-discovery, synchronises GPU-specific labels, and maintains a
consistent view of detected hardware for other Deckhouse modules.

### Key capabilities

- Keeps a per-device representation in `GPUDevice` objects (PCI IDs, MIG
  profiles, memory, compute capability, precision support, management flags).
- Aggregates node-wide state in `GPUNodeState` via readiness conditions
  (for example, `ManagedDisabled`, `InventoryComplete`, `ReadyForPooling`,
  `DriverMissing`, `ToolkitMissing`).
- Emits Kubernetes events (`GPUDeviceDetected`, `GPUInventoryConditionChanged`)
  and Prometheus metrics (`gpu_inventory_devices_total`,
  `gpu_inventory_condition`) for monitoring and alerting.
- Responds to NodeFeature absence or label drift by marking inventory as
  incomplete, ensuring operators are aware when the data pipeline is missing
  inputs.
- Exposes hooks and Helm values to align bootstrap components (GFD with the
  gfd-extender sidecar, DCGM hostengine/exporter, device plugin, MIG manager)
  with module policies.

## Inventory model

- **GPUDevice** – represents a single GPU discovered on a node. The controller
  keeps hardware facts in the status, updates management flags, and triggers
  downstream handlers that apply contracts (auto-attach, health, pools).
- **GPUNodeState** – node-level aggregate with driver/toolkit summary,
  readiness conditions consumed by higher-level controllers and admission
  webhooks.

## Controller workflow

1. Watches `Node` resources along with their `NodeFeature` companion objects
   produced by node-feature-discovery.
2. Builds a deterministic snapshot of GPUs per node: PCI IDs, MIG profile
   counts, memory, compute capability, precision modes, UUIDs.
3. Creates or updates `GPUDevice` objects with owner references to the Node,
   ensuring metadata (labels, inventory ID) and status stay in sync.
4. Removes orphan devices when NodeFeature data disappears or labels are
   cleared, and publishes corresponding events.
5. Reconciles `GPUNodeState` status, updates conditions
   (`ManagedDisabled`, `InventoryComplete`) and records metrics.

## Hooks and bootstrap

The module relies on `module-sdk` hooks to integrate with addon-operator:

- `initialize_values` – prepares value trees and internal sections so chart
  templating stays deterministic.
- `validate_module_config` – sanitises ModuleConfig payloads, applies defaults
  for managed node label, device approval policy, scheduling hints, inventory
  resync interval and the optional HA override, then writes the resulting
  settings into internal values.
- `module_status` – blocks releases when node-feature-discovery is not enabled
  by publishing a `PrerequisiteNotMet` condition and a validation message.
- `discovery_node_feature_rule` – renders the namespace and the NodeFeatureRule that
  labels GPU nodes with the `gpu.deckhouse.io/*` hierarchy.
- `pkg/readiness` – exposes the module readiness probe used by addon-operator to
  block releases while validation errors or pending bootstrap actions remain.

The bootstrap DaemonSets always deploy the NVIDIA stack (GFD + gfd-extender,
DCGM hostengine/exporter, watchdog, validator) on managed nodes regardless of
`monitoring.serviceMonitor`. The monitoring flag only controls whether Prometheus
scrape objects and Grafana dashboards are rendered; DCGM telemetry is exposed via
Prometheus and is not persisted in CRDs.

The DaemonSets are scheduled based on Node labels produced by the shipped
NodeFeatureRule (for example `gpu.deckhouse.io/present=true`) and the managed
nodes policy. They do not depend on `GPUDevice`/`GPUNodeState` existence, so
bootstrap Pods may be running even when the inventory controllers are degraded.

## Packaging and deployment assets

- Helm templates follow Deckhouse patterns: Deployment with HA helpers,
  ServiceAccount/RBAC, metrics Service + ScrapeConfig, and module-managed namespace.
- A pre-delete Job removes dynamic resources (NodeFeatureRule) before Helm wipes the
  release to avoid stale labels after module shutdown.
- `werf.yaml` together with `images/` describes controller, hooks and bundle
  images, enabling reproducible builds under giterminism.
- `openapi/config-values.yaml` and `openapi/values.yaml` expose both public and
  internal schemas for ModuleConfig validation and documentation tooling.

## Requirements

1. A working [Deckhouse Kubernetes Platform](https://deckhouse.io) cluster
   (version 1.71 or newer).
2. The **node-feature-discovery** module (v0.17 or newer) must be enabled;
   hooks guard this prerequisite.
3. GPU nodes have to export hardware information through node-feature-discovery
   so that the provided `NodeFeatureRule` can derive the `gpu.deckhouse.io/*`
   label set.

## Installation

1. Build or download the module images. When building from source use `werf`:

   ```bash
   werf build  # builds controller, hooks and bundle images locally

   # push to a registry and add readable tags like <image>-dev
   MODULES_MODULE_SOURCE=127.0.0.1:5001/gpu-control-plane MODULES_MODULE_TAG=dev make werf-build
   ```

2. Upload images or bundles to the registry used by Deckhouse.
3. Register the module (Deckhouse EE) or deliver the bundle via the modules
   operator.
4. Create a `ModuleConfig` to enable the module. Example with explicit defaults:

   ```yaml
   apiVersion: deckhouse.io/v1alpha1
   kind: ModuleConfig
   metadata:
     name: gpu-control-plane
   spec:
     enabled: true
   settings:
     highAvailability: true
     managedNodes:
       labelKey: gpu.deckhouse.io/enabled
       enabledByDefault: true
     deviceApproval:
       mode: Manual
     scheduling:
       defaultStrategy: Spread
       topologyKey: topology.kubernetes.io/zone
     inventory:
       resyncPeriod: "30s"
   version: 1
   ```

   If a field is omitted the module applies defaults:
   `managedNodes.labelKey=gpu.deckhouse.io/enabled`, `managedNodes.enabledByDefault=true`,
   `deviceApproval.mode=Manual`, `scheduling.defaultStrategy=Spread`,
   `scheduling.topologyKey=topology.kubernetes.io/zone`, `inventory.resyncPeriod=30s`.

5. Confirm that the controller is running in the `d8-gpu-control-plane`
   namespace and that `GPUDevice`/`GPUNodeState` CRs are created for GPU
   nodes.

The background rescan interval can be adjusted via
`.spec.settings.inventory.resyncPeriod` (default `30s`).

## Monitoring

- Prometheus metrics: `gpu_inventory_devices_total`,
  `gpu_inventory_condition{condition=...}`.
- Kubernetes events: `GPUDeviceDetected`, `GPUDeviceRemoved`,
  `GPUInventoryConditionChanged`.
- Optional scraping integration (ScrapeConfig) and PrometheusRule/Grafana dashboards ship
  with the module and are enabled automatically when the required Deckhouse
  modules are present.

## Repository layout

- `openapi/values.yaml` – internal values schema used by hooks and templates.
- `openapi/config-values.yaml` – public schema rendered into documentation.
- `images/hooks/pkg/hooks` – module-sdk hooks compiled into the
  `gpu-control-plane-module-hooks` binary.
- `src/controller` – Go sources of the inventory controller and supporting
  handlers.
- `templates/` – Helm manifests rendered by modules-operator/addon-operator.
- `images/` – `werf.inc.yaml` descriptors for controller, hooks and bundle
  images.

## Configuration reference

- [Config values schema](../openapi/config-values.yaml)
- [Internal values schema](../openapi/values.yaml)
- [Russian config reference](../openapi/doc-ru-config-values.yaml)

```

```
