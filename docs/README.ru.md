<!--
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

---

title: "GPU Control Plane"
menuTitle: "GPU Control Plane"
moduleStatus: Experimental
weight: 10

---

# Модуль GPU Control Plane

Модуль GPU Control Plane добавляет в Deckhouse промышленный слой инвентаризации
и подготовки GPU-узлов. Контроллеры обнаруживают NVIDIA GPU, сохраняют их
профили в пользовательских ресурсах, публикуют диагностические метрики и
подготавливают кластер к дальнейшему планированию AI-нагрузок.

## Краткое описание

Модуль закрывает этап **инвентаризации и диагностики** GPU. Он отслеживает
обновления `Node` и `NodeFeature`, получаемых от node-feature-discovery,
синхронизирует GPU-метки и предоставляет единый источник данных о состоянии
оборудования.

### Основные возможности

- Ведёт учёт каждого устройства в объектах `GPUDevice`: PCI-идентификаторы,
  профили MIG, объём памяти, compute capability, поддерживаемая точность,
  флаги управляемости.
- Агрегирует состояние узла в `GPUNodeState`: сведения о драйвере, готовности
  CUDA Toolkit и условия готовности (например, `ManagedDisabled`,
  `InventoryComplete`, `ReadyForPooling`, `DriverMissing`, `ToolkitMissing`).
- Публикует события Kubernetes (`GPUDeviceDetected`, `GPUInventoryConditionChanged`)
  и метрики Prometheus (`gpu_inventory_devices_total`,
  `gpu_inventory_condition`).
- Корректно реагирует на отсутствие NodeFeature или дрейф меток, помечая
  инвентаризацию как неполную и помогая оперативно выявлять проблемы в цепочке
  данных.
- Управляет bootstrap-компонентами (GFD, DCGM exporter, device plugin,
  MIG manager) через значения Helm и hook'и.

## Модель данных

- **GPUDevice** — отдельное устройство на узле. Контроллер поддерживает
  статус с аппаратными характеристиками, флагами управляемости и вызывает
  обработчики для применения контрактов (авто-привязка, здоровье, пулы).
- **GPUNodeState** — агрегированное состояние узла, включающее драйвер,
  условия готовности и другую информацию для высокоуровневых контроллеров и
  admission webhook'ов.

## Работа контроллера

1. Отслеживает ресурсы `Node` и парные `NodeFeature`, публикуемые
   node-feature-discovery.
2. Формирует детерминированный снимок GPU на узле: PCI ID, профили MIG, память,
   compute capability, доступные режимы точности, UUID.
3. Создаёт или обновляет объекты `GPUDevice`, привязанные к узлу через
   ownerReference, и поддерживает актуальные метки и статус.
4. Удаляет осиротевшие устройства при исчезновении данных из NodeFeature и
   генерирует соответствующие события.
5. Синхронизирует `GPUNodeState`, обновляет условия,
   публикует метрики и гарантирует консистентность данных.

## Hook'и и bootstrap

Интеграция с addon-operator реализована через `module-sdk`:

- `initialize_values` — подготавливает дерево значений, чтобы рендеринг Helm
  оставался детерминированным.
- `validate_module_config` — нормализует ModuleConfig, применяет дефолты для
  метки управляемости, политики подтверждения устройств, подсказок
  планировщику, интервала пересинхронизации и опционального HA, затем
  записывает настройки во внутренние значения.
- `module_status` — блокирует релиз, если модуль node-feature-discovery не
  включён, публикуя условие `PrerequisiteNotMet` и сообщение проверки.
- `discovery_node_feature_rule` — разворачивает пространство имён и NodeFeatureRule,
  которое метит GPU-узлы в пространстве `gpu.deckhouse.io/*`.
- `pkg/readiness` — публикует probe готовности, чтобы addon-operator блокировал
  релизы при ошибках валидации или незавершённом bootstrap.

Bootstrap DaemonSet'ы всегда разворачивают NVIDIA-стек (GFD + gfd-extender, DCGM
hostengine/exporter, watchdog, validator) на управляемых GPU-нодах вне
зависимости от `monitoring.serviceMonitor`. Флаг мониторинга влияет только на
рендер объектов Prometheus/Grafana; DCGM-метрики экспортируются в Prometheus и не
сохраняются в CRD.

Планирование DaemonSet'ов выполняется по меткам Node, которые выставляет
поставляемый NodeFeatureRule (например `gpu.deckhouse.io/present=true`), и
политике управляемых узлов. Они не зависят от наличия `GPUDevice`/`GPUNodeState`,
поэтому bootstrap Pod'ы могут быть запущены даже при деградации контроллеров
инвентаризации.

## Пакетирование и поставка

- Шаблоны Helm используют макросы Deckhouse: Deployment с HA-настройками,
  ServiceAccount/RBAC, сервис метрик + ScrapeConfig, namespace модуля.
- Pre-delete Job перед удалением релиза убирает NodeFeatureRule, чтобы после
  отключения модуля в кластере не оставались устаревшие метки.
- `werf.yaml` и файлы в `images/` описывают образы контроллера, хуков и bundle
  для воспроизводимой сборки под giterminism.
- `openapi/config-values.yaml` и `openapi/values.yaml` предоставляют схемы для
  документации и проверки ModuleConfig.

## Требования

1. Развёрнутый кластер [Deckhouse](https://deckhouse.io) версии не ниже 1.71.
2. Включённый модуль **node-feature-discovery** версии 0.17 или новее. Хуки GPU
   Control Plane проверяют это условие и останавливают установку при его
   нарушении.
3. GPU-узлы должны публиковать информацию через NFD. Поставляемый
   `NodeFeatureRule` добавляет необходимые метки `gpu.deckhouse.io/*`.

## Установка

1. Сборка и публикация образов (при работе с исходным кодом — через werf):

   ```bash
   werf build  # собирает образы контроллера, хуков и bundle локально

   # публикация в реестр с читаемыми тегами вида <image>-dev
   MODULES_MODULE_SOURCE=127.0.0.1:5001/gpu-control-plane MODULES_MODULE_TAG=dev make werf-build
   ```

2. Загрузка образов/бандла в реестр, используемый Deckhouse.
3. Регистрация модуля (для EE-редакции) или доставка бандла через modules-оператор.
4. Создание `ModuleConfig` для включения модуля (пример с явными дефолтами):

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

   При опущенных полях используются значения по умолчанию:
   `managedNodes.labelKey=gpu.deckhouse.io/enabled`, `managedNodes.enabledByDefault=true`,
   `deviceApproval.mode=Manual`, `scheduling.defaultStrategy=Spread`,
   `scheduling.topologyKey=topology.kubernetes.io/zone`, `inventory.resyncPeriod=30s`.

5. Проверка работы: Deployment `gpu-control-plane-controller` в
   пространстве имён `d8-gpu-control-plane`, появление объектов
   `GPUDevice`/`GPUNodeState` для GPU-узлов.

Интервал повторного опроса можно задать через
`.spec.settings.inventory.resyncPeriod` (по умолчанию `30s`).

## Наблюдаемость

- Метрики Prometheus: `gpu_inventory_devices_total`,
  `gpu_inventory_condition{condition=...}`.
- События Kubernetes: `GPUDeviceDetected`, `GPUDeviceRemoved`,
  `GPUInventoryConditionChanged`.
- Ресурсы наблюдаемости (ScrapeConfig, PrometheusRule, Grafana дашборды) поставляются
  модулем и активируются автоматически при наличии необходимых модулей Deckhouse.

## Структура репозитория

- `openapi/values.yaml` — схема внутренних значений, используемых хуками и Helm.
- `openapi/config-values.yaml` — публичная схема для документации ModuleConfig.
- `api/` — Go-типы CRD, общие для компонентов модуля.
- `images/hooks/pkg/hooks` — исходники хуков, собираемых в бинарь
  `gpu-control-plane-module-hooks`.
- `images/gpu-control-plane-artifact` — код контроллера инвентаризации и вспомогательных
  обработчиков.
- `templates/` — Helm-манифесты, которые разворачивает addon-operator.
- `images/` — `werf.inc.yaml` с описанием сборки образов контроллера, хуков и bundle.

```

```
