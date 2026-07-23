#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
SCRIPT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd -P)"
CONTROLLER_GEN_BIN="${CONTROLLER_GEN_BIN:-controller-gen}"

cd "${SCRIPT_ROOT}"
"${CONTROLLER_GEN_BIN}" crd paths=./k8s/pkg/apis/... output:crd:dir=kodata/crds
