# GPU DRA: ручные e2e‑сценарии (v0)

Документ описывает ручные проверки для DRA‑стека GPU (publish/allocate/prepare),
ориентированные на реальную эксплуатацию и референсы NVIDIA DRA/SDN.

## 1. Предусловия

- Kubernetes >= 1.34 (с 1.34 используется отдельный counters‑slice).
- Включены feature gates:
  - `DRAPartitionableDevices`
  - `DRAConsumableCapacity`
  - `DRAExtendedResource`
  - `DRADeviceBindingConditions`
- Установлены драйвер NVIDIA, NVML, доступен `nvidia-smi`.
- Запущены компоненты модуля:
  - `gpu-handler`, `gpu-node-agent`, `gpu-dra-controller`.
- На ноде есть GPU с поддержкой MIG (для сценариев MIG).
- Включён CDI (по умолчанию `/etc/cdi`), в cluster runtime — containerd.

## 2. Инвентарь кластера (факт)

```bash
kubectl get physicalgpus.gpu.deckhouse.io -o wide
```

Найдено:

- `k8s-w1-gpu.apiac.ru`: NVIDIA GeForce RTX 3090 Ti, label `gpu.deckhouse.io/device=geforce-rtx-3090-ti`, MIG: нет
- `k8s-w2-gpu.apiac.ru`: NVIDIA GeForce RTX 3090, label `gpu.deckhouse.io/device=geforce-rtx-3090`, MIG: нет
- `k8s-w3-gpu.apiac.ru`: NVIDIA A30 PCIe, label `gpu.deckhouse.io/device=a30-pcie`, MIG: да

## 2.1 Блокеры на момент проверки (факт)

- Pod webhook `pod.gpu-control-plane.deckhouse.io` не отвечает по пути `/<validate|mutate>-v1-pod`.
  Контроллер регистрирует `/validate--v1-pod` и `/mutate--v1-pod` (двойной дефис),
  а манифесты содержат одиночный дефис. В `gpu-e2e` получение GPU‑подов падает:
  `failed calling webhook "pod.gpu-control-plane.deckhouse.io": the server could not find the requested resource`.
- Обойтись без namespace label нельзя: `ValidatingAdmissionPolicy gpu-control-plane-gpu-enabled-namespace`
  запрещает GPU‑ресурсы в namespace без `gpu.deckhouse.io/enabled=true`.
- В ResourceSlice должны быть префиксные ключи атрибутов
  (`gpu.deckhouse.io/migProfile`, `gpu.deckhouse.io/gpuUUID`, `gpu.deckhouse.io/ccMajor`,
  `gpu.deckhouse.io/ccMinor`, `gpu.deckhouse.io/driverVersion`). Если в кластере видны
  старые ключи (`mig_profile`, `gpu_uuid`, `cc_major`, `cc_minor`), это блокер: модуль не обновлён.

## 3. Переменные окружения (пример)

```bash
export NS=d8-gpu-control-plane
export NODE_3090TI=k8s-w1-gpu.apiac.ru
export NODE_3090=k8s-w2-gpu.apiac.ru
export NODE_A30=k8s-w3-gpu.apiac.ru
export DEV_3090TI=geforce-rtx-3090-ti
export DEV_3090=geforce-rtx-3090
export DEV_A30=a30-pcie
export ER_3090TI=gpu.deckhouse.io/rtx3090ti
export ER_3090=gpu.deckhouse.io/rtx3090
export ER_A30=gpu.deckhouse.io/a30
export DRIVER=gpu.deckhouse.io
```

## 4. Базовые проверки до сценариев

### 4.1 Компоненты

```bash
kubectl -n ${NS} get pods -o wide | egrep "gpu-handler|gpu-node-agent|gpu-dra-controller"
```

### 4.2 ResourceSlice / DeviceClass / ResourceClaim

```bash
kubectl get resourceslices.resource.k8s.io -o wide
kubectl get deviceclasses.resource.k8s.io -o wide
kubectl get resourceclaims.resource.k8s.io -A -o wide
kubectl get physicalgpus.gpu.deckhouse.io -o wide
```

### 4.3 CDI

```bash
kubectl -n ${NS} get ds gpu-handler -o yaml | rg -n "cdi|CDI"
# На ноде:
# ls -la /etc/cdi
```

### 4.4 MIG (если применимо)

```bash
# На ноде:
nvidia-smi mig -lgip
```

## 5. Матрица сценариев по картам

- **RTX 3090 Ti (k8s-w1-gpu)**: S1, S2, S4, S5.
- **RTX 3090 (k8s-w2-gpu)**: S1, S2, S4, S5.
- **A30 (k8s-w3-gpu)**: S1, S2, S3, S4, S5, S6, S7.

## 6. Сценарии

### S1. ExtendedResource → физическая карта (Exclusive) для всех карт

**Цель:** проверить основной UX v0: DeviceClass + extended resource → автоклейм.

**Шаги:**

```yaml
# deviceclass-physical.yaml (3 класса под все карты)
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-rtx-3090-ti-physical
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical" &&
          device.attributes["gpu.deckhouse.io"].device == "geforce-rtx-3090-ti"
  extendedResourceName: gpu.deckhouse.io/rtx3090ti
---
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-rtx-3090-physical
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical" &&
          device.attributes["gpu.deckhouse.io"].device == "geforce-rtx-3090"
  extendedResourceName: gpu.deckhouse.io/rtx3090
---
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-physical
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical" &&
          device.attributes["gpu.deckhouse.io"].device == "a30-pcie"
  extendedResourceName: gpu.deckhouse.io/a30
```

```bash
kubectl apply -f deviceclass-physical.yaml
```

```yaml
# pod-extended-physical.yaml (3 pod под все карты)
apiVersion: v1
kind: Pod
metadata:
  name: cuda-physical-3090ti
spec:
  nodeSelector:
    kubernetes.io/hostname: k8s-w1-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        requests:
          gpu.deckhouse.io/rtx3090ti: "1"
        limits:
          gpu.deckhouse.io/rtx3090ti: "1"
---
apiVersion: v1
kind: Pod
metadata:
  name: cuda-physical-3090
spec:
  nodeSelector:
    kubernetes.io/hostname: k8s-w2-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        requests:
          gpu.deckhouse.io/rtx3090: "1"
        limits:
          gpu.deckhouse.io/rtx3090: "1"
---
apiVersion: v1
kind: Pod
metadata:
  name: cuda-physical-a30
spec:
  nodeSelector:
    kubernetes.io/hostname: k8s-w3-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        requests:
          gpu.deckhouse.io/a30: "1"
        limits:
          gpu.deckhouse.io/a30: "1"
```

**Проверки:**

```bash
kubectl get pod cuda-physical-3090ti cuda-physical-3090 cuda-physical-a30 -o wide
kubectl get resourceclaims.resource.k8s.io -A -o wide
kubectl get resourceclaims.resource.k8s.io -A -o yaml | rg -n "cuda-physical|gpu.deckhouse.io"
kubectl exec -it cuda-physical-3090ti -- nvidia-smi -L
kubectl exec -it cuda-physical-3090 -- nvidia-smi -L
kubectl exec -it cuda-physical-a30 -- nvidia-smi -L
```

Ожидаемое:

- Scheduler создал special ResourceClaim.
- В `ResourceClaim.status.allocation` есть `results` для `gpu.deckhouse.io`.
- В Pod виден GPU.

Факт:

- В namespace `gpu-e2e` (label включён) создание подов падает из‑за pod webhook:
  `failed calling webhook "pod.gpu-control-plane.deckhouse.io": the server could not find the requested resource`.
- В namespace без label создание подов с GPU запрещено политикой `gpu-control-plane-gpu-enabled-namespace`.

### S2. ResourceClaim → Physical (без extended resource) для всех карт

**Цель:** ручное создание Claim и привязка в Pod.

```yaml
# claim-physical.yaml (3 claim под все карты)
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-physical-3090ti
spec:
  devices:
    requests:
      - name: gpu
        exactly:
          deviceClassName: nvidia-rtx-3090-ti-physical
          count: 1
---
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-physical-3090
spec:
  devices:
    requests:
      - name: gpu
        exactly:
          deviceClassName: nvidia-rtx-3090-physical
          count: 1
---
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-physical-a30
spec:
  devices:
    requests:
      - name: gpu
        exactly:
          deviceClassName: nvidia-a30-physical
          count: 1
```

```yaml
# pod-claim-physical.yaml (3 pod под все карты)
apiVersion: v1
kind: Pod
metadata:
  name: cuda-claim-physical-3090ti
spec:
  resourceClaims:
    - name: gpu
      resourceClaimName: claim-physical-3090ti
  nodeSelector:
    kubernetes.io/hostname: k8s-w1-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: gpu
---
apiVersion: v1
kind: Pod
metadata:
  name: cuda-claim-physical-3090
spec:
  resourceClaims:
    - name: gpu
      resourceClaimName: claim-physical-3090
  nodeSelector:
    kubernetes.io/hostname: k8s-w2-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: gpu
---
apiVersion: v1
kind: Pod
metadata:
  name: cuda-claim-physical-a30
spec:
  resourceClaims:
    - name: gpu
      resourceClaimName: claim-physical-a30
  nodeSelector:
    kubernetes.io/hostname: k8s-w3-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: gpu
```

**Проверки:**

```bash
kubectl apply -f claim-physical.yaml
kubectl apply -f pod-claim-physical.yaml
kubectl get resourceclaim claim-physical-3090ti -o yaml
kubectl get resourceclaim claim-physical-3090 -o yaml
kubectl get resourceclaim claim-physical-a30 -o yaml
kubectl exec -it cuda-claim-physical-3090ti -- nvidia-smi -L
kubectl exec -it cuda-claim-physical-3090 -- nvidia-smi -L
kubectl exec -it cuda-claim-physical-a30 -- nvidia-smi -L
```

### S3. MIG partitionable devices (KEP‑4815) для A30

**Цель:** проверить MIG‑оферы, sharedCounters и consumesCounters.

**Шаги:**

```yaml
# deviceclass-mig-2g12.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-mig-2g12
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "MIG" &&
          device.attributes["gpu.deckhouse.io"].device == "a30-pcie" &&
          device.attributes["gpu.deckhouse.io"].migProfile == "2g.12gb"
```

```yaml
# claim-mig.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-mig-2g12
spec:
  devices:
    requests:
      - name: mig
        exactly:
          deviceClassName: nvidia-a30-mig-2g12
          count: 1
```

```yaml
# pod-claim-mig.yaml
apiVersion: v1
kind: Pod
metadata:
  name: cuda-claim-mig
spec:
  resourceClaims:
    - name: mig
      resourceClaimName: claim-mig-2g12
  nodeSelector:
    kubernetes.io/hostname: k8s-w3-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: mig
```

Повторять этот сценарий только на A30 (MIG‑узел).

**Проверки:**

```bash
kubectl get resourceslices.resource.k8s.io -o yaml | rg -n "sharedCounters|consumesCounters|migProfile"
kubectl get resourceclaim claim-mig-2g12 -o yaml
kubectl exec -it cuda-claim-mig -- nvidia-smi -L
```

Ожидаемое:

- Для 1.34+: отдельный counters‑slice и devices‑slice.
- У MIG‑устройств `consumesCounters` ссылается на `sharedCounters` того же pool/generation.

### S4. TimeSlicing (KEP‑5075) через ResourceClaimTemplate для всех карт

**Цель:** проверить sharePercent + time‑slicing для Physical.

```yaml
# deviceclass-timeslicing.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-rtx-3090-ti-timeslicing
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical" &&
          device.attributes["gpu.deckhouse.io"].device == "geforce-rtx-3090-ti"
  config:
    - opaque:
        driver: gpu.deckhouse.io
        parameters:
          apiVersion: resource.gpu.deckhouse.io/v1alpha1
          kind: GpuConfig
          sharing:
            strategy: TimeSlicing
            timeSlicingConfig:
              interval: Long
```

```yaml
# rct-timeslicing.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: rct-timeslicing-3090ti
spec:
  spec:
    devices:
      requests:
        - name: gpu
          exactly:
            deviceClassName: nvidia-rtx-3090-ti-timeslicing
            count: 1
            capacity:
              requests:
                sharePercent: "50"
```

```yaml
# pod-timeslicing.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: v1
kind: Pod
metadata:
  name: cuda-timeslicing-3090ti
spec:
  resourceClaims:
    - name: gpu
      resourceClaimTemplateName: rct-timeslicing-3090ti
  nodeSelector:
    kubernetes.io/hostname: k8s-w1-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: gpu
```

Повторить для остальных карт:

- RTX 3090: заменить `geforce-rtx-3090-ti` → `geforce-rtx-3090`, имена на `*-3090`, `nodeSelector` на `k8s-w2-gpu.apiac.ru`.
- A30: заменить `geforce-rtx-3090-ti` → `a30-pcie`, имена на `*-a30`, `nodeSelector` на `k8s-w3-gpu.apiac.ru`.

**Проверки:**

```bash
kubectl get resourceclaim -o yaml | rg -n "sharePercent|consumedCapacity"
# На ноде:
nvidia-smi -i 0 --query-compute-apps=pid --format=csv
nvidia-smi compute-policy -i 0
```

### S5. MPS (Physical) для всех карт

**Цель:** проверить MPS‑daemon и CDI‑инъекцию env/mounts.

```yaml
# deviceclass-mps.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-rtx-3090-ti-mps
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical" &&
          device.attributes["gpu.deckhouse.io"].device == "geforce-rtx-3090-ti"
  config:
    - opaque:
        driver: gpu.deckhouse.io
        parameters:
          apiVersion: resource.gpu.deckhouse.io/v1alpha1
          kind: GpuConfig
          sharing:
            strategy: MPS
            mpsConfig:
              defaultActiveThreadPercentage: 50
```

```yaml
# rct-mps.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: rct-mps-3090ti
spec:
  spec:
    devices:
      requests:
        - name: gpu
          exactly:
            deviceClassName: nvidia-rtx-3090-ti-mps
            count: 1
            capacity:
              requests:
                sharePercent: "50"
```

```yaml
# pod-mps.yaml (пример для RTX 3090 Ti; повторить для RTX 3090 и A30)
apiVersion: v1
kind: Pod
metadata:
  name: cuda-mps-3090ti
spec:
  resourceClaims:
    - name: gpu
      resourceClaimTemplateName: rct-mps-3090ti
  nodeSelector:
    kubernetes.io/hostname: k8s-w1-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command:
        ["bash", "-lc", "env | rg CUDA_MPS; mount | rg nvidia-mps; sleep 3600"]
      resources:
        claims:
          - name: gpu
```

Повторить для остальных карт:

- RTX 3090: заменить `geforce-rtx-3090-ti` → `geforce-rtx-3090`, имена на `*-3090`, `nodeSelector` на `k8s-w2-gpu.apiac.ru`.
- A30: заменить `geforce-rtx-3090-ti` → `a30-pcie`, имена на `*-a30`, `nodeSelector` на `k8s-w3-gpu.apiac.ru`.

**Проверки:**

```bash
kubectl exec -it cuda-mps-3090ti -- sh -lc 'env | rg CUDA_MPS'
kubectl exec -it cuda-mps-3090ti -- sh -lc 'mount | rg nvidia-mps'
# Для остальных карт: cuda-mps-3090, cuda-mps-a30
```

### S6. MPS (MIG) для A30

**Цель:** проверить MPS‑шаринг для MIG‑оферов.

```yaml
# deviceclass-mig-mps.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-mig-2g12-mps
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "MIG" &&
          device.attributes["gpu.deckhouse.io"].device == "a30-pcie" &&
          device.attributes["gpu.deckhouse.io"].migProfile == "2g.12gb"
  config:
    - opaque:
        driver: gpu.deckhouse.io
        parameters:
          apiVersion: resource.gpu.deckhouse.io/v1alpha1
          kind: MigDeviceConfig
          sharing:
            strategy: MPS
            mpsConfig:
              defaultActiveThreadPercentage: 50
```

```yaml
# rct-mig-mps.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: rct-mig-mps
spec:
  spec:
    devices:
      requests:
        - name: mig
          exactly:
            deviceClassName: nvidia-a30-mig-2g12-mps
            count: 1
            capacity:
              requests:
                sharePercent: "50"
```

```yaml
# pod-mig-mps.yaml
apiVersion: v1
kind: Pod
metadata:
  name: cuda-mig-mps
spec:
  resourceClaims:
    - name: mig
      resourceClaimTemplateName: rct-mig-mps
  nodeSelector:
    kubernetes.io/hostname: k8s-w3-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command:
        ["bash", "-lc", "env | rg CUDA_MPS; mount | rg nvidia-mps; sleep 3600"]
      resources:
        claims:
          - name: mig
```

### S7. Динамическая нарезка MIG + негативные кейсы (A30)

**Цель:** показать жизненный цикл MIG: карта без нарезки → нарезка по claim → очистка → перенарезка, плюс проверить ошибки.

**0. Стартовое состояние (карта без нарезки):**

```bash
kubectl get physicalgpus.gpu.deckhouse.io k8s-w3-gpu-apiac-ru-0-10de-20b7 -o jsonpath='{.status.currentState.nvidia.mig.mode}{"\n"}'
# На ноде:
nvidia-smi mig -lgi
```

Ожидаемое: `mode: Disabled`, GI/CI отсутствуют.

**1. Нарезка на 1g.6gb (4 инстанса максимум):**

```yaml
# deviceclass-mig-1g6.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-mig-1g6
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "MIG" &&
          device.attributes["gpu.deckhouse.io"].device == "a30-pcie" &&
          device.attributes["gpu.deckhouse.io"].migProfile == "1g.6gb"
```

```yaml
# claim-mig-1g6.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-mig-1g6
spec:
  devices:
    requests:
      - name: mig
        exactly:
          deviceClassName: nvidia-a30-mig-1g6
          count: 1
```

```yaml
# pod-mig-1g6.yaml
apiVersion: v1
kind: Pod
metadata:
  name: cuda-mig-1g6
spec:
  resourceClaims:
    - name: mig
      resourceClaimName: claim-mig-1g6
  nodeSelector:
    kubernetes.io/hostname: k8s-w3-gpu.apiac.ru
  containers:
    - name: app
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["bash", "-lc", "nvidia-smi -L; sleep 3600"]
      resources:
        claims:
          - name: mig
```

**Проверки:**

```bash
kubectl apply -f deviceclass-mig-1g6.yaml
kubectl apply -f claim-mig-1g6.yaml
kubectl apply -f pod-mig-1g6.yaml
kubectl get resourceclaim claim-mig-1g6 -o yaml | rg -n "allocation|migProfile"
# На ноде:
nvidia-smi mig -lgi
nvidia-smi mig -lci
```

Ожидаемое: появляется GI/CI под профиль 1g.6gb.

**2. Очистка (Unprepare) и проверка, что нарезка ушла:**

```bash
kubectl delete pod cuda-mig-1g6 --ignore-not-found
kubectl delete resourceclaim claim-mig-1g6 --ignore-not-found
# На ноде:
nvidia-smi mig -lgi
nvidia-smi mig -lci
```

Ожидаемое: GI/CI удалены (MIG mode может остаться Enabled, это допустимо).

**3. Перенарезка на 2g.12gb:**

```yaml
# deviceclass-mig-2g12.yaml (из S3)
# claim-mig-2g12.yaml (из S3)
# pod-claim-mig.yaml (из S3)
```

```bash
kubectl apply -f deviceclass-mig-2g12.yaml
kubectl apply -f claim-mig-2g12.yaml
kubectl apply -f pod-claim-mig.yaml
# На ноде:
nvidia-smi mig -lgi
nvidia-smi mig -lci
```

Ожидаемое: появляются GI/CI под профиль 2g.12gb.

**4. Негативные кейсы:**

4.1. Запрос MIG на карте без MIG (RTX 3090):

```yaml
# deviceclass-rtx3090-mig.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-rtx-3090-mig
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "MIG" &&
          device.attributes["gpu.deckhouse.io"].device == "geforce-rtx-3090"
```

```yaml
# claim-rtx3090-mig.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-rtx3090-mig
spec:
  devices:
    requests:
      - name: mig
        exactly:
          deviceClassName: nvidia-rtx-3090-mig
          count: 1
```

Ожидаемое: `ResourceClaim` остаётся без `status.allocation`.

4.2. Несуществующий профиль:

```yaml
# deviceclass-mig-bad.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-mig-bad
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "MIG" &&
          device.attributes["gpu.deckhouse.io"].device == "a30-pcie" &&
          device.attributes["gpu.deckhouse.io"].migProfile == "3g.20gb"
```

```yaml
# claim-mig-bad.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-mig-bad
spec:
  devices:
    requests:
      - name: mig
        exactly:
          deviceClassName: nvidia-a30-mig-bad
          count: 1
```

Ожидаемое: аллокация не происходит, claim без `status.allocation`.

4.3. Переполнение по инстансам:

```yaml
# claim-mig-over.yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaim
metadata:
  name: claim-mig-over
spec:
  devices:
    requests:
      - name: mig
        exactly:
          deviceClassName: nvidia-a30-mig-2g12
          count: 3
```

Ожидаемое: аллокация не происходит (maxInstances=2 для 2g.12gb).

4.4. GPU занят (GPUFreeCheck):

```bash
# Запускаем физический pod на A30 и держим его запущенным (из S1: cuda-physical-a30).
kubectl get pod cuda-physical-a30 -o wide
# Затем пытаемся создать MIG claim:
kubectl apply -f claim-mig-2g12.yaml
kubectl apply -f pod-claim-mig.yaml
kubectl -n ${NS} logs -l app=gpu-handler --since=10m | rg -n "GPU|busy|Prepare"
```

Ожидаемое: Prepare отклоняется с ошибкой занятости GPU.

### S8. DeviceStatus (KEP‑4817)

**Цель:** проверить запись `ResourceClaim.status.devices`.

```bash
kubectl get resourceclaim <claim> -o yaml | rg -n "status:|devices:|conditions:"
```

Ожидаемое:

- Для подготовленных устройств выставлены `Ready` + binding conditions.
- После Unprepare записи удаляются.

### S9. VFIO (опционально, пока не готово)

**Цель:** проверить VFIO‑поток и отметить блокеры.

Сейчас в `nvcdi` нет CDI‑подготовки для VFIO.
Ожидаемый результат — отказ на Prepare (vfio prepare is not implemented).

### S10. Валидация webhook (негативный тест)

**Цель:** проверить, что неверные параметры отклоняются webhook’ом.

```yaml
# deviceclass-invalid.yaml
apiVersion: resource.k8s.io/v1
kind: DeviceClass
metadata:
  name: nvidia-a30-invalid
spec:
  selectors:
    - cel:
        expression: |
          device.attributes["gpu.deckhouse.io"].vendor == "nvidia" &&
          device.attributes["gpu.deckhouse.io"].deviceType == "Physical"
  config:
    - opaque:
        driver: gpu.deckhouse.io
        parameters:
          apiVersion: resource.gpu.deckhouse.io/v1alpha1
          kind: GpuConfig
          sharing:
            strategy: TimeSlicing
            timeSlicingConfig:
              interval: "UltraFast"
```

Ожидаемое: `kubectl apply -f deviceclass-invalid.yaml` завершается ошибкой валидации.

## 7. Проверка ошибок/ивентов

```bash
kubectl get events -A --sort-by=.lastTimestamp | rg -n "ResourceSlice|FeatureGate|NVML|Toolkit"
kubectl -n ${NS} logs -l app=gpu-handler --since=10m | rg -n "ResourceSlice|Prepare|Unprepare"
kubectl -n ${NS} logs -l app=gpu-dra-controller --since=10m | rg -n "allocate|claim|deviceclass"
```

## 8. Очистка

```bash
kubectl delete pod cuda-physical-3090ti cuda-physical-3090 cuda-physical-a30 \
  cuda-claim-physical-3090ti cuda-claim-physical-3090 cuda-claim-physical-a30 \
  cuda-claim-mig cuda-mig-1g6 cuda-timeslicing-3090ti cuda-mps-3090ti cuda-mig-mps --ignore-not-found
kubectl delete resourceclaim claim-physical-3090ti claim-physical-3090 claim-physical-a30 \
  claim-mig-1g6 claim-mig-2g12 claim-rtx3090-mig claim-mig-bad claim-mig-over --ignore-not-found
kubectl delete resourceclaimtemplate rct-timeslicing-3090ti rct-mps-3090ti rct-mig-mps --ignore-not-found
kubectl delete deviceclass nvidia-rtx-3090-ti-physical nvidia-rtx-3090-physical nvidia-a30-physical \
  nvidia-a30-mig-1g6 nvidia-a30-mig-2g12 nvidia-rtx-3090-ti-timeslicing nvidia-rtx-3090-ti-mps \
  nvidia-a30-mig-2g12-mps nvidia-rtx-3090-mig nvidia-a30-mig-bad nvidia-a30-invalid --ignore-not-found

# Если создавали варианты для RTX 3090/A30 в S4/S5:
# kubectl delete pod cuda-timeslicing-3090 cuda-timeslicing-a30 cuda-mps-3090 cuda-mps-a30 --ignore-not-found
# kubectl delete resourceclaimtemplate rct-timeslicing-3090 rct-timeslicing-a30 rct-mps-3090 rct-mps-a30 --ignore-not-found
# kubectl delete deviceclass nvidia-rtx-3090-timeslicing nvidia-a30-timeslicing nvidia-rtx-3090-mps nvidia-a30-mps --ignore-not-found
```
