## План bootstrap-компонентов

> Шпаргалка, чтобы не отходить от паттернов Deckhouse (`ee/modules/040-node-manager`) и
> virtualization при переносе NVIDIA утилит.

### Общие принципы

- **Werf**: для каждого образа (gfd, dcgm, dcgm-exporter) повторяем цепочку `*-src-artifact` →
  `*-build-artifact` → финальный `distroless`, как это сделано в
  `nvidia-device-plugin`, `nvidia-dcgm`, `nvidia-dcgm-exporter`.
- **Patched upstream**: исходники подтягиваем из официальных репозиториев (см. таблицу ниже),
  патчи кладём в `images/<name>/patches` с тем же форматом, что у node-manager.
- **Security**: используем только готовые `helm_lib_*`-хелперы
  (`pod_security_context_run_as_user_root`, `module_container_*`, `helm_lib_tolerations`)
  и не добавляем openshift-специфику.
- **Managed nodes**: все DaemonSet'ы должны брать `nodeSelector`/`tolerations` через
  `gpuControlPlane.managedNode*`-хелперы, без жёстко зашитых таинтов.
- **Validator toggles**: init-контейнеры `nvidia-fs` и `gdrcopy` отключены по умолчанию и включаются
  только если заданы значения `bootstrap.validator.enableNvidiaFSValidation` /
  `bootstrap.validator.enableGDRCopyValidation`.

### Источники и версии

| Компонент             | Апстрим                                                                  | Базовый пример                  | Примечание                                              |
| --------------------- | ------------------------------------------------------------------------ | ------------------------------- | ------------------------------------------------------- |
| Validator             | `github.com/NVIDIA/gpu-operator` (`cmd/nvidia-validator`)                | (уже добавлен)                  | Версия `v25.3.4`, build из `golang-bookworm`.           |
| GPU Feature Discovery | `github.com/NVIDIA/gpu-feature-discovery` (через `nvidia-device-plugin`) | `nvidia-gpu/gfd.yml`            | Требуются init/sidecar контейнеры `config-manager`.     |
| DCGM                  | `NVIDIA/dcgm-exporter` (бинарь `dcgmproftester`, `nv-hostengine`)        | `nvidia-gpu/dcgm.yaml`          | Нужно volume `/run/nvidia/driver`, hostPID, privileged. |
| DCGM Exporter         | тот же репозиторий, бинарь `dcgm-exporter`                               | `nvidia-gpu/dcgm-exporter.yaml` | Плюс Service+ServiceMonitor.                            |

### Этапы работ

1. **GPU Feature Discovery**

   - Создать образы `gpu-gfd-src-artifact`/`gpu-gfd-build-artifact`/`gpu-gfd`.
   - Template: DaemonSet + (опционально) VPA по примеру `nvidia-gpu/gfd.yml`.
   - Значения: только тюнинг (`sleepInterval`, `featuresDir`, `failOnInitError`, `hostSysPath`); включение/отключение — через `ModuleConfig.spec.settings.bootstrap.gfd.enabled`, placement — через `gpuControlPlane.managedNode*`-хелперы (label-гейт по managed GPU‑нодам).

2. **DCGM daemon**

   - Образы `gpu-dcgm` аналогично `nvidia-dcgm`.
   - Helm: DaemonSet с hostPID, `nvidia` volume mounts, ConfigMap с `nv-hostengine` args.
   - Значения: стратегия MIG (`single`, `mixed`), флаги запуска `dcgmproftester`.

3. **DCGM exporter**

   - Отдельный образ c distroless.
   - Helm: Deployment/DaemonSet + Service + ServiceMonitor (используем `templates/gpu-control-plane-controller/service-monitor.yaml` как reference).
   - Значения: порты, labels, аннотации ServiceMonitor; включение/отключение — через `ModuleConfig.spec.settings.bootstrap.dcgmExporter.enabled` (и общие настройки мониторинга модуля).

4. **Связка с bootstrap-контроллером**

   - Контроллер следит за готовностью bootstrap‑ворклоадов и пишет только conditions в `GPUNodeState.status.conditions`
     (например: `DriverMissing`, `ToolkitMissing`, `MonitoringMissing`, `GFDReady`, `ReadyForPooling`).
   - Helm‑шаблоны **не зависят** от `GPUNodeState.status.*` для рендера: bootstrap DaemonSet’ы рендерятся по `ModuleConfig`,
     а placement выполняется через `gpuControlPlane.managedNode*` (label‑гейт на managed GPU‑нодах).

5. **Интеграция с Inventory**

   - GFD класть факты в `node-feature-discovery` директорию (`/etc/kubernetes/...`),
     контроллер считывает лейблы `gpu.deckhouse.io/*`.
   - DCGM exporter → новые метрики (state gauges, ошибочные события) + Grafana панели.

6. **Проверка**
   - Для каждого компонента добавить helm-render тесты (через `werf render` + `kubeconform`).
   - Минимальные e2e-сценарии: локальный kind + fake GPU (исп. `nvidia-device-plugin` из
     node-manager).

### Мониторинг и дашборды

- **Prometheus**: Bootstrap-контроллер экспортирует собственные метрики
  (`gpu_bootstrap_node_phase{node,phase}`, `gpu_bootstrap_condition{node,condition}`,
  `gpu_bootstrap_handler_errors_total{handler}`), которые дополняют существующие
  `gpu_inventory_*`. Эти метрики становятся основой для оповещений и Grafana.
- **Grafana**: в `monitoring/grafana-dashboards/main` лежат три куратора
  (`gpu-inventory`, `gpu-node`, `gpu-workloads`). Они повторяют паттерн
  virtualization/SDS: основной обзор, детализация по узлу и по workload'ам.
  Дашборды потребляют как контроллерские метрики, так и DCGM (узловой телеметрии)
  и kube-state-metrics (GPU requests).

Этот план покрывает требования пользователя: все образы собираются как в `040-node-manager`,
манифесты не содержат OpenShift-специфики, а безопасность/тейнты управляются только через
наши значения модуля.
