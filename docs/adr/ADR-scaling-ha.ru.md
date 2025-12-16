# ADR: Масштабирование и HA (1000 namespaces / 1000 pools)

## Контекст

Целевой сценарий нагрузки:

- до `~1000` namespaces (тенантов),
- до `~1000` пулов в кластере,
- до `~100` GPU на пул (порядка `~100k` объектов `GPUDevice`).

Ограничения/инварианты интерфейса (зафиксировано ранее):

- Аннотации назначения устройств:
  - `gpu.deckhouse.io/assignment=<poolName>` (namespaced `GPUPool`)
  - `cluster.gpu.deckhouse.io/assignment=<poolName>` (cluster `ClusterGPUPool`)
- Ресурсы, которые запрашивают workload’ы:
  - `gpu.deckhouse.io/<poolName>` (namespaced `GPUPool`)
  - `cluster.gpu.deckhouse.io/<poolName>` (cluster `ClusterGPUPool`)
- **StatefulSet запрещён** (никаких стабильных ordinal/identity через Stateful).
- Архитектурные ориентиры: kubebuilder/controller-runtime паттерны, SOLID/clean code, практики virtualization; module-sdk учитываем (логи/метрики/конфиг).

## Проблема

При кратном росте числа пулов/устройств текущие подходы становятся узкими местами:

1. **O(cluster) reconcile‑логика.** Любые `List(all Pods)`/`List(all GPUDevices)` внутри reconcile пула умножаются на количество пулов и превращаются в лавинообразную нагрузку на cache/apiserver/CPU.

2. **per-pool DaemonSet модель не масштабируется.** Если на каждый пул рендерить отдельный `DaemonSet` (device-plugin/validator/MIG manager), то при сотнях/тысячах пулов получаем взрыв по количеству объектов/Pod’ов и контроллеров DaemonSet’ов. Это становится лимитом раньше, чем любая оптимизация кода control-plane.

3. **Admission blast radius.** Глобальные fail‑closed webhooks на Pod’ы повышают риск «остановки кластера» при деградации webhook’а.

## Решение (финальное)

### 1) Контроллеры: leader-only (как в kubebuilder/virtualization)

Контроллеры (`inventory/bootstrap/pool`) остаются **leader-only** через manager leader election:

- это стандартный и наиболее безопасный паттерн controller-runtime,
- это устраняет гонки по одним и тем же объектам и write‑amplification,
- это экономит память: cache на 100k `GPUDevice` не дублируется на N активных reconcile‑инстансах ради «линейного масштабирования».

Масштабирование обеспечивается не количеством активных лидеров, а:

- устранением O(cluster) операций,
- индексами/селективными list,
- контролируемой concurrency (`MaxConcurrentReconciles`) и лимитами REST‑клиента (`QPS/Burst`),
- выносом высоко‑QPS компонент (webhooks) в масштабируемую плоскость.

### 2) Webhooks/metrics: multi-replica и всегда активны

Admission webhooks в controller-runtime **не требуют leader election** и обслуживаются всеми репликами процесса. Для выдерживания нагрузки и повышения доступности:

- допускается горизонтальное масштабирование deployment’а (реплики реально работают),
- blast radius ограничивается selectors (в первую очередь namespace‑гейтом `gpu.deckhouse.io/enabled=true`),
- Pod webhooks не должны быть кластер‑глобальными без необходимости: **mutating и validating Pod webhooks** ограничиваем `namespaceSelector`, а правила «Pod с GPU‑ресурсами разрешён только в enabled namespace» выносим в **ValidatingAdmissionPolicy (CEL)**, потому что selectors не умеют матчить `resources.limits`.
- отказоустойчивость webhooks не должна блокировать весь кластер (fail‑policy на Pod webhook’ах должен быть обоснован и ограничен по scope).

### 3) Пулы: детерминированная capacity + pod-derived usage (только observability)

Финальная семантика:

- `pool-controller` считает **детерминированную** `status.capacity.total` из состава устройств пула и настроек нарезки (`unit/slices/MIG`).
- `status.capacity.used/available` — **информационные** поля для UI/наблюдаемости:
  - `used` считается как сумма запросов (`resources.*`) у **уже назначенных на ноды** Pod’ов, которые запрашивают ресурс пула;
  - `available = max(0, total-used)`;
  - эти значения могут лагать и **не являются** «истиной» для планировщика.
- admission **не выполняет** динамическую проверку «available прямо сейчас», но делает **статические** проверки:
  - запрошен не более чем один pool‑ресурс (один Pod → один пул),
  - namespace включён (гейт),
  - запрошено не больше, чем `pool.status.capacity.total` (если total уже известен).

Как избегаем O(cluster):

- mutating Pod webhook добавляет лейблы `gpu-control-plane.deckhouse.io/pool` + `gpu-control-plane.deckhouse.io/pool-scope`;
- cache менеджера подписывается на Pod’ы **во всех namespace’ах** только по label‑selector (а не на весь кластер);
- отдельные лёгкие usage‑контроллеры пересчитывают `used/available` по Pod’ам **только своего пула**.

Причина: динамическая «доступность» и фактическое выделение — зона scheduler/device-plugin/ResourceQuota. В control-plane нам нужны лишь детерминированные инварианты и прозрачный статус.

### 4) DevicePlugin backend: единый node-agent вместо per-pool DaemonSet

Чтобы выдержать сотни/тысячи пулов при сохранении интерфейса `gpu.deckhouse.io/<poolName>`:

- вводится **единый node-agent DaemonSet** на GPU‑нодах,
- node-agent отвечает за node‑side:
  - применение MIG/time-slicing (по pool spec),
  - регистрацию device-plugin ресурсов для пулов, присутствующих на ноде,
  - валидацию устройств ноды и репорт состояния.

`gpupool-controller` перестаёт рендерить per-pool workloads и становится чистым control-plane (выбор устройств + обновление статусов/conditions).

Практическая реализация регистрации ресурсов:

- kubelet device-plugin регистрируется **по одному ресурсу на один unix-socket**; один процесс может держать несколько сокетов и регистрировать несколько ресурсов.
- node-agent в одном Pod поднимает N gRPC endpoints (N сокетов) — **по одному на pool‑ресурс**, который реально присутствует на этой ноде.
- N ограничено сверху числом физических GPU на ноде, потому что одно устройство принадлежит ровно одному пулу (assignment/poolRef), поэтому на одной ноде физически не может «встретиться» 1000 пулов.

#### Когда выкатывается node-agent и как он обновляется

- `gpu-node-agent` — **базовый DaemonSet фазы bootstrap** (разворачивается вместе с модулем) и запускается только на управляемых GPU‑нодах через `nodeAffinity`/label‑гейт.
- До появления назначений (`GPUDevice.status.poolRef`) node-agent работает в «idle» режиме: не регистрирует pool‑ресурсы и не применяет MIG/time-slicing.
- Изменения назначений/конфигурации подхватываются **без рестартов Pod’а**: node-agent смотрит `GPUDevice`/`GPUPool`/`ClusterGPUPool`, пересчитывает локальную конфигурацию и применяет её идемпотентно (с debounce), перерегистрируя только затронутые ресурсы/сокеты.
- Критичные для node‑side поля (`spec.backend`, `spec.resource.*`) считаются **immutable**: изменение требует пересоздания пула, чтобы избежать флаппинга и сложных «переездов» устройств между протоколами/режимами.

#### Смешивание backend’ов на одной ноде

- Смешивание разных backend’ов (например `DevicePlugin` + `DRA` + `HAMi`) на **одной и той же ноде** не поддерживаем: на ноде должен быть ровно один активный backend‑engine, иначе велик риск конфликтов по MIG/time-slicing и протоколам kubelet.
- Ограничение обеспечивается организационно (node labels + `GPUPool.spec.nodeSelector`) и условиями/Events при конфликтных назначениях.

### 5) Недвусмысленность: имена пулов уникальны в кластере

Так как аннотация назначения содержит только `<poolName>` (без namespace), для исключения конкуренции контроллеров и неоднозначности:

- `GPUPool`/`ClusterGPUPool` должны быть **уникальны по имени в пределах кластера**.
- validating webhook обязан это обеспечивать.

Дополнительно для адресного доступа и работы node-agent мы расширяем внутреннюю ссылку:

- `GPUDevice.status.poolRef` содержит минимально необходимые поля для однозначного обращения к пулу: `name` + `namespace` для `GPUPool` (для `ClusterGPUPool` `namespace` пустой). Внешний интерфейс (имена ресурсов/аннотаций) не меняется.

### 6) Расширяемость: HAMi / Volcano / DRA

Для балансирования функциональности и масштаба мы разделяем «внешний контракт» (CRD + ресурсные имена) и «engine» реализации:

- **DevicePlugin (базовый путь)**: `gpu-node-agent` регистрирует `gpu.deckhouse.io/<pool>` / `cluster.gpu.deckhouse.io/<pool>` и реализует MIG/time-slicing локально на ноде.
- **DRA (будущее расширение)**: backend‑engine на node‑side, где фактическое выделение/инъекция делается через DRA/ResourceClaims. Внешний контракт для пользователя сохраняется: Pod по‑прежнему задаёт `resources.limits`/`resources.requests` `gpu.deckhouse.io/<pool>`, а admission добавляет/преобразует необходимые поля Pod’а под DRA (детали реализации фиксируются отдельным ADR при внедрении).
- **Project HAMi / Volcano (будущее расширение)**: альтернативные engines для vGPU/шеринга (memory/SM/cores) и/или batch scheduling. Интеграция возможна в двух формах:
  1. как отдельный backend/engine (явный выбор в пуле/ModuleConfig, возможен `schedulerName: volcano` и дополнительные протоколы),
  2. как переиспользование node-side логики внутри `gpu-node-agent` при сохранении внешнего контракта ресурсов (без обязательного scheduler extender).

## Последствия

Плюсы:

- выдерживаем рост по пулам/устройствам за счёт устранения квадратичных операций и отказа от per-pool DaemonSet модели,
- минимизируем риски гонок: один лидер — один источник истины для reconcile,
- webhooks масштабируются горизонтально и не зависят от лидерства.

Минусы/стоимость:

- node-agent — отдельная существенная подсистема (образ, RBAC, протокол получения конфигурации),
- необходимо переопределить семантику pool status и admission (`used/available` — только observability; отказ от динамических проверок «available прямо сейчас»),
- потребуется удалить per-pool workload рендеринг из control-plane после внедрения node-agent (без требований совместимости).

## План внедрения (без совместимости)

1. Довести pool reconcile и admission до «статической» семантики (без O(cluster) `List Pods`; `used/available` — только через label‑filtered Pod cache, без влияния на admission).
2. Зафиксировать минимальный CRD (без телеметрии) и выровнять контроллеры/вебхуки под него.
3. Внедрить `gpu-node-agent` и перенести на него node‑side (регистрация ресурсов, MIG/time-slicing, валидатор).
4. Удалить per-pool DaemonSet/ConfigMap рендеринг из контроллеров.
