# GPUPool: режимы, бэкенды и требования

## Вендор и бэкенды

- `provider`: сейчас только `nvidia`.
- `backend`: `device-plugin` (по умолчанию) или `dra`.
  - `device-plugin` доступен на всех поддерживаемых версиях k8s.
  - `dra` доступен только при k8s >= 1.32 и включённых фичах DRA; иначе webhook пула отклоняет.

## Режимы аллокации

- `mode=Card` поддерживается в обоих бэкендах.
- `mode=MIG`:
  - `device-plugin`: поддерживаем MIG и MIG+TimeSlicing (переподписка через `slicesPerUnit`, гибкая аллокация в нашем DP).
  - `dra`: только статически нарезанные MIG (без динамической нарезки и без time-slicing/oversubscription; ограничение драйвера).
- `slicesPerUnit` (переподписка) задаётся всегда, дефолт = 1 (эксклюзив). При `backend=dra` и/или `mode=MIG` в DRA переподписка запрещена.
- MPS не закладываем: отсутствует изоляция, требования к hostIPC/UID, лимит клиентов; оставляем только Time Slicing как базовую переподписку.

## Правила селектора/иммутабельности пула

- Иммутабельны после создания: `resource`, `allocation` (mode, migProfile, slicesPerUnit, maxDevicesPerNode), `deviceSelector`, `nodeSelector`, `backend`, `provider`.
- Селектор устройств (include/exclude): `inventoryIDs`, `products`, `pciVendors`, `pciDevices`, `migCapable`, `migProfiles`.
- Валидации пула:
  - `slicesPerUnit >= 1`; `backend=dra` → `slicesPerUnit` должен быть 1; при `mode=MIG` в DRA — только migCapable=true и профиль поддерживается статически.
  - `backend=dra` → версия кластера >=1.32, фича DRA включена.

## Admission и мутации Pod

- Общие проверки: доступ (namespaces/SA/DexGroups), запрос ресурса только для существующего пула, отсутствие Faulted/Ignore устройств, нода не InfraDegraded/HasFaulted.
- Для `device-plugin`:
  - Pod использует `resources.{requests,limits}.gpu.deckhouse.io/<pool>`.
  - Мутация: добавляем nodeSelector/nodeAffinity на узлы пула, tolerations под пуловый taint, topologySpreadConstraints при стратегии Spread. BinPack — без дополнительных хинтов.
  - Разрешаем переподписку: `slicesPerUnit` > 1 → DP публикует слоты; внутри ноды DP стремится разложить слоты по разным картам (spread-first).
- Для `dra`:
  - Pod не задаёт GPU-ресурсы; вместо этого добавляется `resourceClaims` с ссылкой на `ResourceClaimTemplate` пула (один шаблон на пул).
  - Мутация: те же affinity/tolerations/spread-хинты, чтобы ограничить размещение на узлы пула.
  - Запрещаем одновременное использование GPU-ресурса и DRA-клейма одного пула.

## Контроллеры/объекты

- `device-plugin` backend:
  - Пер-пул DaemonSet device-plugin с AllowedDevices, учитывающий `slicesPerUnit` и MIG профиль. MIG-manager общий на ноду с картой PCI→профиль.
- `dra` backend:
  - Генерируем `ResourceClass` и `ResourceClaimTemplate` под пул (Card). MIG допускаем только статический (без переподписки).
  - Полагаться на upstream DRA kubelet plugin; аллокацию/prepare выполняет драйвер NVIDIA.

## Риски и ограничения

- MPS не используем (нет изоляции, требования hostIPC/UID, лимит клиентов).
- DRA сегодня не умеет динамический MIG и oversubscription; для этих сценариев остаёмся на device-plugin.
- Переподписка (Time Slicing) — только через `device-plugin`, с явным контролем числа слотов (`slicesPerUnit`).

## Потоки и последовательности

### Device-Plugin backend (Card/MIG, с переподпиской)

```mermaid
sequenceDiagram
    participant User as Пользователь
    participant API as API Server
    participant PoolCtrl as GPUPool Controller
    participant DP as Per-pool DevicePlugin
    participant Kubelet as Kubelet

    User->>API: Создаёт GPUPool (provider=nvidia, backend=dp, mode=Card/MIG, slicesPerUnit)
    PoolCtrl-->>API: Валидирует (immutability, slicesPerUnit>=1)
    PoolCtrl-->>DP: Генерирует/обновляет ConfigMap AllowedDevices (+mig-manager конфиг), перезапуск DP
    User->>API: Создаёт Pod с resources.gpu.deckhouse.io/<pool>
    PoolCtrl/Admission->>API: Мутирует Pod (affinity/tolerations/spread)
    API->>Kubelet: Под назначен на ноду пула
    Kubelet->>DP: Allocate/GetPreferredAllocation
    DP-->>Kubelet: Выдаёт ID слотов (spread-first по картам, затем stack)
    Kubelet-->>Pod: CDI/device nodes прокинуты
```

### DRA backend (Card, без переподписки)

```mermaid
sequenceDiagram
    participant User
    participant API
    participant PoolCtrl as GPUPool Controller
    participant DRA as ResourceClass/Claim Template
    participant Kubelet
    participant DRAPlugin as NVIDIA DRA kubelet plugin

    User->>API: Создаёт GPUPool (provider=nvidia, backend=dra, mode=Card, slicesPerUnit=1)
    PoolCtrl-->>API: Валидирует (k8s>=1.32, DRA enabled, no oversub, no dynamic MIG)
    PoolCtrl-->>DRA: Создаёт ResourceClass + ResourceClaimTemplate для пула
    User->>API: Создаёт Pod (без GPU ресурсов)
    Admission->>API: Мутирует Pod, добавляет resourceClaims: template=<pool>, affinity/tolerations/spread
    API->>Kubelet: Под назначен на ноду пула
    Kubelet->>DRAPlugin: PrepareResourceClaims
    DRAPlugin-->>Kubelet: Готовит устройства (только статические GPU/MIG)
```

### Поток MIG (DP) с переподпиской

```mermaid
flowchart LR
    Pool["GPUPool (backend=dp, mode=MIG, slicesPerUnit>=1, migProfile)"]
    Ctrl["GPUPool Controller"]
    MIGMgr["nvidia-mig-manager (общий на ноду)"]
    DP["Per-pool DevicePlugin"]
    Pod["Pod с ресурсом пула"]

    Pool --> Ctrl
    Ctrl --> MIGMgr["Конфиг PCI->migProfile"]
    MIGMgr --> DP["Партиции готовы"]
    Ctrl --> DP["AllowedDevices (партиции) x slicesPerUnit"]
    Pod --> DP["Allocate: spread по партициям, затем stack"]
```
