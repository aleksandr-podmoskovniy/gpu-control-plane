## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 001-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (remove DisableResourceNamingInConfig), so custom resource names from ConfigMap are applied by the device-plugin.

### 002-enable-resource-renaming.patch

Same as 001 but rebased to v0.18.0 source (DisableResourceNamingInConfig becomes no-op).
