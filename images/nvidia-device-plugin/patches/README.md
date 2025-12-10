## Patches

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 002-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (make DisableResourceNamingInConfig a no-op) so custom resource names from ConfigMap are applied by the device-plugin (rebased to v0.18.0 source).

### 003-custom-resource-prefix.patch

Switch the default resource prefix to `gpu.deckhouse.io` and stop adding the default `gpu` resource when custom resources are provided (guard defaults for MIG too).

### 004-gpu-uuid-pattern-match.patch

Allow GPU resource patterns to match either product name or GPU UUID. This enables a strict allowlist per device by setting `resources.gpus[].pattern` to a specific UUID.

### 005-resource-prefix-override.patch

Make the resource prefix overridable via `NVIDIA_RESOURCE_PREFIX` env var so per-pool device-plugin can publish either `gpu.deckhouse.io/*` or `cluster.gpu.deckhouse.io/*` without rebuilding the image.
