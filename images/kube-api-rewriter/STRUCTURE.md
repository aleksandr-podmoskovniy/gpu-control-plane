# Project Structure

```
images/kube-api-rewriter
├── cmd/kube-api-rewriter      # entry point wiring logging, metrics and HTTP servers
├── pkg
│   ├── gpu                    # module-specific rewrite rules
│   ├── labels                 # helper to pass request metadata through contexts
│   ├── log                    # slog setup used by the binary
│   ├── monitoring             # health, metrics and pprof handlers
│   ├── proxy                  # HTTP proxy implementation + Prometheus metrics
│   ├── rewriter               # request/response rewrite scaffolding
│   ├── server                 # reusable HTTP server runners
│   └── target                 # Kubernetes client configuration helpers
├── local                      # utilities for manual testing (manifests, Dockerfile, kubeconfig)
├── METRICS.md                 # exported metrics reference
├── STRUCTURE.md               # (this file) high-level overview
├── Taskfile.dist.yaml         # convenience commands for local builds
├── go.mod/go.sum              # standalone Go module definition
├── mount-points.yaml          # containerd strict-mode mount points
└── werf.inc.yaml              # werf image build description reused by the module
```

The proxy is built as a standalone Go module to keep its dependency list
isolated from controller and hooks code. Production builds are driven by
`werf.inc.yaml`, while `Taskfile.dist.yaml` and files under `local/` help when
debugging the proxy outside of the Deckhouse build pipeline.
