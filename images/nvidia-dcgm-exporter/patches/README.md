## Patchess

### 000-go-mod.patch

Bump libraries versions to resolve CVE

### 001-dcp-metrics.patch

Enables additional NVLink/ECC/throttling/DCP counters in `dcp-metrics-included.csv` so exporter ships richer GPU telemetry out of the box.

### 002-reduce-prof-metrics.patch

Comments out profiling-only metrics that are frequently unsupported on hosts (profiling/DCGM modules not loaded), to avoid noisy "metric not enabled" warnings in exporter logs while keeping the rest of the expanded metrics set.
