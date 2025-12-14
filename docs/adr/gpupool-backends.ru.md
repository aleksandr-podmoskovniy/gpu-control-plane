# GPUPool: провайдеры и бэкенды

## См. также

- `docs/adr/ADR-scaling-ha.ru.md` — масштабирование/HA и целевая эволюция реализации (node-agent вместо per-pool DaemonSet).

## Решение

### Термины (фиксируем, без двусмысленностей)

- `spec.provider` — вендор/семейство node-side реализации (инвентарь/валидатор/MIG и т. п.). Сейчас поддерживаем только `Nvidia`.
- `spec.backend` — **node-side engine** (реализация внутри `gpu-node-agent`) для выдачи ресурса. Внешний контракт для пользователя остаётся прежним:
  - `DevicePlugin` — базовый путь: пользователь задаёт `resources.limits`/`resources.requests` для `gpu.deckhouse.io/<poolName>` / `cluster.gpu.deckhouse.io/<poolName>`.
  - `DRA` — будущий путь: пользователь по‑прежнему задаёт те же `resources.*`, а admission добавляет/преобразует Pod под DRA/ResourceClaims (детали фиксируются отдельным ADR при внедрении).

### Инварианты интерфейса (фиксируем)

- **Один пул → один ресурс.** Имя ресурса всегда детерминировано от `metadata.name` пула:
  - `GPUPool`: `gpu.deckhouse.io/<poolName>`
  - `ClusterGPUPool`: `cluster.gpu.deckhouse.io/<poolName>`
- Для namespaced пула **не добавляем namespace** в имя ресурса (namespace известен из Pod’а).
- Из-за `*/assignment=<poolName>` **имена пулов уникальны в кластере** (между namespace’ами и между `GPUPool`/`ClusterGPUPool`) — enforcing через validating webhook.
- Oversubscription/time-slicing описывается **только** через `spec.resource.slicesPerUnit` (никаких дополнительных ресурсов на пул).
- Для `unit=MIG` пул описывает **один MIG профиль** (`spec.resource.migProfile`), без `migLayout` и без смешивания профилей внутри одного пула.
- На одной ноде допускается ровно **один backend‑engine**: смешивание `DevicePlugin`/`DRA`/`HAMi` на одной и той же ноде не поддерживаем; выбираем node-pool’ами и `nodeSelector`.

- Поддерживаем адаптерный подход: контроллер пула делегирует провайдер/бэкенд‑специфику отдельным адаптерам.
- Целевая реализация для `Nvidia`+`DevicePlugin`: **единый node-agent DaemonSet** на GPU‑нодах, который:
  - применяет MIG/time-slicing конфигурацию из `GPUPool/ClusterGPUPool`,
  - регистрирует в kubelet device-plugin ресурсы `gpu.deckhouse.io/<pool>` и `cluster.gpu.deckhouse.io/<pool>` для пулов, присутствующих на ноде,
  - запускает/проверяет валидатор для устройств ноды и репортит статусы в CR.
- Текущее состояние кода: per-pool `DaemonSet` (device-plugin/validator/MIG manager) рендерится контроллером. Это функционально для небольшого числа пулов, но **не масштабируется** до сотен/тысяч пулов и будет удалено после внедрения node-agent.
- Для других провайдеров/бэкендов контроллер ставит condition `Supported=False` с причиной `UnsupportedProvider`/`UnsupportedBackend` и не пытается готовить конфиги/DaemonSet; при смене backend/provider выполняется cleanup per-pool ресурсов.
- Логика подбора устройств универсальна: матчится по PCI/vendor/MIG/labels, устройства назначаются в пул через аннотацию `gpu.deckhouse.io/assignment=<pool>`, capacity учитывает `slicesPerUnit`, `maxDevicesPerNode` и MIG профили.
- Тайм-шеринг описывается `unit=Card` + `slicesPerUnit>1`; `slicesPerUnit=1` трактуется как эксклюзив. Для MIG аналогично: `unit=MIG` + `slicesPerUnit`.
- Селекторы устройств в `GPUPool.spec.deviceSelector` неизменяемые: mutating webhook запрещает изменение после создания, чтобы не «переезжали» устройства между пулами.
- По умолчанию включены per-pool taints/affinity (`taintsEnabled=true`), admission добавляет tolerations/affinity к Pod; можно отключить на пуле.

## План расширения

- Для нового провайдера: добавить адаптер, который умеет строить конфиги и деплоить нужный плагин (и при необходимости мониторинг/диагностику); валидатор пула должен проверить допустимые поля.
- Для DRA бэкенда: отдельный адаптер, работающий с ResourceClass/Claims, без `nvidia-device-plugin`. Важно: DRA‑пулы **не используют** `resources.limits` и не требуют «доступности прямо сейчас» в admission; заявка/выделение — зона DRA scheduler/driver.
- Для advanced GPU sharing/нарезки: предусмотреть интеграции с внешними проектами:
  - **Project HAMi**: vGPU/memory/SM hard limits, scheduler extender + device-plugin. Варианты интеграции: (а) отдельный engine/режим (явно включаемый), (б) переиспользовать его node-side логику внутри `gpu-node-agent`, сохраняя внешний контракт `gpu.deckhouse.io/<pool>`.
  - **Volcano**: batch/gang scheduling и vGPU device-plugin (volcano-vgpu). Варианты: (а) отдельный engine/режим (может требовать `schedulerName: volcano`), (б) использовать только Volcano scheduler для non-GPU задач, не влияя на DevicePlugin backend.

## Ограничения

- Сейчас любые значения provider/backend, отличные от (Nvidia, DevicePlugin), помечаются condition’ом `Supported=False`; дальнейшая логика (конфиги, DaemonSet) не выполняется.
- MIG и аппаратная диагностика завязаны на NVIDIA (NVML/DCGM); для других вендоров потребуется новая цепочка сбора железных атрибутов и мониторинга.
- Масштабирование по количеству пулов для DevicePlugin бэкенда достигается только через внедрение node-agent; до этого per-pool DaemonSet модель ограничена по практическим лимитам кластера (количество DS/Pod’ов/суммарный размер объектов).
- Полноценная поддержка vGPU (память/SM/cores как в HAMi/Volcano) потребует либо отдельного backend’а с собственной семантикой запроса, либо минимального расширения API пула (строго обоснованного) и явного выбора engine на пуле/глобально.
