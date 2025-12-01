## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 002-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (make DisableResourceNamingInConfig a no-op) so custom resource names from ConfigMap are applied by the device-plugin (rebased to v0.18.0 source).

### 003-custom-resource-prefix.patch

Switch the default resource prefix to `gpu.deckhouse.io` and stop adding the default `gpu` resource when custom resources are provided (guard defaults for MIG too).

### 004-blank-import-klog.patch

Upstream `api/config/v1/config.go` keeps a klog import but no references after our rename change. Convert it to a blank import to satisfy the compiler when resource-renaming is enabled.
