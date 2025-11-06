# Metrics

All metrics are exported via `/metrics` endpoint on the monitoring listener (see
`METRICS_LISTEN_ADDR` in `cmd/kube-api-rewriter/main.go`). The namespace is
`deckhouse_gpu`, subsystem `kube_api_proxy`.

## `deckhouse_gpu_kube_api_proxy_requests_total`

Total number of HTTP requests processed by the proxy. A request is counted once,
after it finishes with either success or error.

Labels:

- `handler` – logical instance name (for controllers we use `gpu-api`).
- `resource` – Kubernetes resource resolved from the request path.
- `verb` – uppercased HTTP method (`GET`, `POST`, …).
- `watch` – either `watch` or `regular`, depending on the query string.
- `result` – `success` when the upstream call succeeded, `error` otherwise.

## `deckhouse_gpu_kube_api_proxy_request_duration_seconds`

Histogram with the duration of completed proxy calls. Same label set as
`requests_total` but without `result`.

## `deckhouse_gpu_kube_api_proxy_bytes_total`

Counter with the total amount of data transferred through the proxy. The metric
uses the following labels in addition to the common ones (`handler`, `resource`,
`verb`, `watch`):

- `direction` – one of:
  - `from_client` — bytes read from the controller request body;
  - `to_target` — bytes forwarded to the upstream API server;
  - `from_target` — bytes read from the upstream API server;
  - `to_client` — bytes written back to the controller.

The metric is only increased when a positive payload is observed, so it is safe
to rely on the absence of a series in dashboards and alerts.
