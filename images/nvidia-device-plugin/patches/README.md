## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 002-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (make DisableResourceNamingInConfig a no-op) so custom resource names from ConfigMap are applied by the device-plugin (rebased to v0.18.0 source).
