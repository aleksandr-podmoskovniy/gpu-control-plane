# Модуль GPU Control Plane

Стек автоматизации, который обнаруживает GPU-оборудование в кластере
Deckhouse, публикует объекты инвентаризации и подготавливает узлы к работе
GPU control plane. Репозиторий повторяет паттерны модулей `virtualization` и
`ai-playground`: контроллеры на базе controller-runtime, go‑hooks поверх
module-sdk, Helm-шаблоны через `deckhouse_lib_helm` и пайплайн сборки `werf`.

## Текущее состояние

- Контроллер инвентаризации следит за `Node` и `NodeFeature`, создаёт ресурсы
  `GPUDevice` / `GPUNodeState`, экспортирует метрики Prometheus и события
  Kubernetes.
- Go‑хуки нормализуют настройки модуля, доставляют обязательный
  `NodeFeatureRule` и выставляют статусные Conditions модуля.
- Поставка идентична другим модулям Deckhouse: образы собираются как артефакты
  (`controller`, `hooks`), bundle образ содержит шаблоны и OpenAPI схемы.
  OpenAPI описывает базовые переключатели ModuleConfig (метка управляемых
  узлов, политика утверждения устройств, подсказки планировщику, интервал
  пересинхронизации inventory, флаг HA).

## Требования

- **Кластер Deckhouse** версии 1.68+ с заполненной ClusterConfiguration.
- **Модуль Node Feature Discovery** (v0.17 и новее) должен быть включён: hook
  `module_status` переведёт модуль в `PrerequisiteNotMet`, если NFD отсутствует.
- Узлы с GPU обязаны публиковать топологию через NFD; модуль сам создаёт
  нужный `NodeFeatureRule`.

## Сборка и публикация образов

```bash
# Подготовить инструменты (golangci-lint, module-sdk wrapper и т.д.)
make ensure-tools

# Собрать все образы модуля (hooks, controller, bundle) локально
werf build

# Собрать и запушить образы в реестр (добавит читаемые теги вида <image>-dev)
MODULES_MODULE_SOURCE=127.0.0.1:5001/gpu-control-plane MODULES_MODULE_TAG=dev make werf-build
```

`werf-giterminism.yaml` фиксирует контекст сборки; проверка выполняется
командой `make ensure-tools`.

## Развёртывание в Deckhouse

1. Запушьте образы и bundle в реестр, который использует ваш Deckhouse.
2. Зарегистрируйте модуль (для EE) или загрузите bundle через Deckhouse
   modules operator.
3. Включите модуль через `ModuleConfig`, например:

   ```yaml
   apiVersion: deckhouse.io/v1alpha1
   kind: ModuleConfig
   metadata:
     name: gpu-control-plane
   spec:
     enabled: true
     settings:
       highAvailability: true
       managedNodes:
         labelKey: gpu.deckhouse.io/enabled
         enabledByDefault: true
       deviceApproval:
         mode: Manual
       scheduling:
         defaultStrategy: Spread
         topologyKey: topology.kubernetes.io/zone
       inventory:
         resyncPeriod: "30s"
     version: 1
   ```

   Значения по умолчанию соответствуют примеру выше, поэтому поля можно не
   указывать.

4. Убедитесь, что контроллер запущен в `d8-gpu-control-plane`, и что для
   GPU-узлов появляются ресурсы `GPUDevice`/`GPUNodeState`.

> ℹ️ Bootstrap-компоненты (GFD с сайдкаром gfd-extender, DCGM hostengine +
> exporter, watchdog/validator) разворачиваются автоматически на всех
> управляемых узлах вне зависимости от настройки `monitoring.serviceMonitor`.
> Отключение мониторинга влияет только на публикацию ServiceMonitor и Grafana,
> но не на сбор health-данных внутри `GPUDevice`.

## Разработка

```bash
# Линтеры + форматирование
make lint

# Unit-тесты (хуки + контроллер)
make test

# Полный цикл (lint + tests + kubeconform + пр.)
make verify

# Только тесты контроллера
cd images/gpu-control-plane-artifact && go test ./... -cover
```

Дополнительная developer-документация размещается в директории `docs/`
(`*.md` на EN/RU).

## Мониторинг

- **Прометеус-метрики**: inventory и bootstrap публикают гейджи `gpu_inventory_*`
  и `gpu_bootstrap_*`, в том числе фазы (`gpu_bootstrap_node_phase`) и условия
  (`gpu_bootstrap_condition`). Эти метрики пригодны для алертов Deckhouse.
- **Grafana**: в `monitoring/grafana-dashboards/main` доступны три дашборда
  (`GPU Control Plane / Overview`, `/Node`, `/Workloads`). Они соответствуют
  паттернам virtualization/SDS: обзорная панель, детализация по узлу (DCGM) и
  рабочим нагрузкам (namespace/pod).
