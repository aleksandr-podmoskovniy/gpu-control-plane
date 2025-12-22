# Component versions

This directory keeps a single source of truth for component versions that are
referenced during the build pipeline and documentation generation.

- `versions.yml` is loaded in `.werf/consts.yaml`; the values are propagated to
  werf templates so images and docs stay in sync.
- The `firmware` section tracks NVIDIA stack versions (driver, CUDA toolkit,
  DCGM) used by the inventory controller handlers.
- The `core` section records major Go dependencies that influence CRD and API
  compatibility.
- The `package` section lists tooling required for development and CI (Go toolchain,
  module-sdk, Kubernetes API, linters).

Update this file whenever the module upgrades runtime dependencies or tooling.
