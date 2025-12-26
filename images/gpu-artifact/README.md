# gpu-artifact

Source tree for GPU module binaries. This directory is built by werf and produces the following
binaries:

- gpu-controller
- gpu-node-agent
- gpu-dra-controller
- gpu-dra-plugin

Deployment manifests and CRDs for the module live at the repository root (`templates/`, `crds/`).
Local CRD generation output is written to `crds/` at the repository root.

## Local development

```sh
make generate
make manifests
make build
make test
```
