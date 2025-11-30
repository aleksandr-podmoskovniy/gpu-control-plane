## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 001-enable-resource-renaming.patch

Keep `config.Resources` and sharing overrides intact (remove DisableResourceNamingInConfig), so custom resource names from ConfigMap are applied by the device-plugin.
