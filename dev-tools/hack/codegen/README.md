# Kubernetes Code Generation

This directory keeps the Kubernetes code generation entrypoints and their pinned Go tool imports together.

- `tools.go` tracks generator dependencies for Go modules.
- `kube_codegen.sh` provides the shared Kubernetes codegen functions.
- `update-codegen.sh` regenerates typed clients and helper code.
- `controller-gen.sh` regenerates CRDs with `controller-gen`.
- `boilerplate.go.txt` is the generated Go file header.
