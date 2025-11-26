# GPUPool: провайдеры и бэкенды

## Решение

- Поддерживаем адаптерный подход: контроллер пула делегирует провайдер/бэкенд‑специфику отдельным адаптерам.
- На текущем этапе реализован только провайдер `Nvidia` с бэкендом `DevicePlugin`: per-pool nvidia-device-plugin + nvidia-mig-manager (если `unit=MIG`). Конфиги/DaemonSet рендерит контроллер, Helm‑шаблоны не используются.
- Для других провайдеров/бэкендов контроллер ставит condition `Supported=False` с причиной `UnsupportedProvider`/`UnsupportedBackend` и не пытается готовить конфиги/DaemonSet; при смене backend/provider выполняется cleanup per-pool ресурсов.
- Логика подбора устройств универсальна: матчится по PCI/vendor/MIG/labels, устройства назначаются в пул через аннотацию `gpu.deckhouse.io/assignment=<pool>`, capacity учитывает `slicesPerUnit`, `maxDevicesPerNode` и MIG профили.
- Тайм-шеринг описывается `unit=Card` + `slicesPerUnit>1` (или `timeSlicingResources[]`); `slicesPerUnit=1` трактуется как эксклюзив. Для MIG аналогично: `slicesPerUnit`/`MIGLayout[*].profiles[*].slicesPerUnit`.
- Селекторы устройств в `GPUPool.spec.deviceSelector` неизменяемые: mutating webhook запрещает изменение после создания, чтобы не «переезжали» устройства между пулами.
- По умолчанию включены per-pool taints/affinity (`taintsEnabled=true`), admission добавляет tolerations/affinity к Pod; можно отключить на пуле.

## План расширения

- Для нового провайдера: добавить адаптер, который умеет строить конфиги и деплоить нужный плагин/health/telemetry; валидатор пула должен проверить допустимые поля.
- Для DRA бэкенда: отдельный адаптер, работающий с ResourceClass/Claims и scheduler hook’ами, без nvidia-device-plugin. Пока отложено до появления стабильного драйвера и поддержки oversubscription в DRA.

## Ограничения

- Сейчас любые значения provider/backend, отличные от (Nvidia, DevicePlugin), помечаются condition’ом `Supported=False`; дальнейшая логика (конфиги, DaemonSet) не выполняется.
- MIG/health/telemetry завязаны на NVIDIA (DCGM/NVML); для других вендоров потребуется новая цепочка сбора железных атрибутов и health.
