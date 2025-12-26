# GPU Control Plane Module

Automation stack that discovers GPU hardware in a Deckhouse cluster, publishes
inventory objects, and prepares nodes for the GPU control plane. The repository
follows the same delivery patterns as the `virtualization` and `ai-playground`
modules: controller-runtime based controllers, module-sdk hooks, Helm templates
powered by `deckhouse_lib_helm`, and a `werf` build pipeline.

## Features (current stage)

- Inventory controller that watches `Node` + `NodeFeature`, creates
  `GPUDevice`/`GPUNodeState` resources, emits Prometheus metrics and
  Kubernetes events.
- Hooks that normalise module settings, deliver a mandatory
  `NodeFeatureRule`, and expose module status conditions.
- Helm/werf packaging identical to other Deckhouse modules: images are built as
  artifacts (`controller`, `hooks`), a bundle image stores rendered templates
  and OpenAPI schemas.
  OpenAPI schemas document a small set of ModuleConfig switches (managed node
  label, device approval policy, scheduling hints, inventory resync interval,
  HA override).

## Prerequisites

- **Deckhouse cluster** (>= 1.68) with ClusterConfiguration populated.
- **Node Feature Discovery module** (v0.17 or newer) must be enabled. The
  `module_status` hook marks the module as `PrerequisiteNotMet` and blocks
  deployment if `node-feature-discovery` is absent.
- Nodes that should be inventoried must expose GPU topology via NFD (the module
  ships a `NodeFeatureRule` automatically).

## Building and publishing images

```bash
# Prepare build-time tools (golangci-lint, module-sdk wrapper, etc.)
make ensure-tools

# Build all module images (hooks, controller, bundle) locally
werf build

# Build and push images into a registry (adds readable tags like <image>-dev)
MODULES_MODULE_SOURCE=127.0.0.1:5001/gpu-control-plane MODULES_MODULE_TAG=dev make werf-build
```

The `werf-giterminism.yaml` file locks down the build context; fuzziness is
checked by `make ensure-tools`.

## Deploying into a Deckhouse cluster

1. Upload images/bundle to the registry used by your Deckhouse edition.
2. Register the module (EE-style) or push the bundle via the Deckhouse modules
   operator.
3. Enable the module with a `ModuleConfig` resource. Example with defaults:

   ```yaml
   apiVersion: deckhouse.io/v1alpha1
   kind: ModuleConfig
   metadata:
     name: gpu-control-plane
   spec:
    enabled: true
    settings:
      highAvailability: true        # force HA regardless of control-plane topology
      managedNodes:
          labelKey: gpu.deckhouse.io/enabled
          enabledByDefault: true
        deviceApproval:
          mode: Manual                # or Automatic / Selector
        scheduling:
          defaultStrategy: Spread
          topologyKey: topology.kubernetes.io/zone
      inventory:
        resyncPeriod: "0s"
    version: 1
   ```

````

 Defaults (when fields are omitted) match the example above:
 `managedNodes.labelKey=gpu.deckhouse.io/enabled`, `managedNodes.enabledByDefault=true`,
 `deviceApproval.mode=Manual`, `scheduling.defaultStrategy=Spread`,
 `scheduling.topologyKey=topology.kubernetes.io/zone`, `inventory.resyncPeriod=0s`.

When the module is enabled, the hook ensures that NFD is present and creates
the `NodeFeatureRule` required for GPU discovery.

4. Verify that the controller is running in the `d8-gpu-control-plane`
 namespace and that `GPUDevice`/`GPUNodeState` objects appear for GPU
 nodes.

> ℹ️ The module currently focuses on inventory/diagnostics. Bootstrap
> controllers (GFD/DCGM launch, MIG configuration) are brought in the next
> stages.

## Development workflow

```bash
# Static analysis + formatting
make lint

# Unit tests (hooks + controller)
make test

# Full CI equivalent (lint + tests)
make verify

# Controller tests only
cd images/gpu-control-plane-artifact && go test ./... -cover
````

Additional developer documentation (both EN/RU) is located in `docs/README.md`
and `docs/README.ru.md`.
