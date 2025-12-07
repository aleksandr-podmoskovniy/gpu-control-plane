# GPU validator patches

## 001-support-custom-resource-name.patch

Разрешает передавать кастомное имя GPU-ресурса через `NVIDIA_RESOURCE_NAME`, чтобы валидатор корректно работал с пер-пуловыми ресурсами `gpu.deckhouse.io/*` и `cluster.gpu.deckhouse.io/*`.
