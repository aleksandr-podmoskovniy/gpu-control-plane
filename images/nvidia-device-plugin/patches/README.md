## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 002-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (make DisableResourceNamingInConfig a no-op) so custom resource names from ConfigMap are applied by the device-plugin (rebased to v0.18.0 source).

### 003-custom-resource-prefix.patch

Switch the default resource prefix to `gpu.deckhouse.io` and stop adding the default `gpu` resource when custom resources are provided (guard defaults for MIG too).
