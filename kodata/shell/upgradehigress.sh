#!/bin/bash
set -euo pipefail

VERSION="${1:-${VERSION:-2.1.6}}"
KO_DATA_PATH="${KO_DATA_PATH:-./kodata}"
CHART_PATH="${KO_DATA_PATH}/charts/higress-${VERSION}.tgz"

if [ ! -f "$CHART_PATH" ]; then
  echo "higress chart not found: $CHART_PATH" >&2
  exit 1
fi

helm upgrade higress "$CHART_PATH" \
  --namespace higress-system \
  --create-namespace \
  --version "v${VERSION}" \
  --set global.ingressClass=higress \
  --set higress-core.gateway.replicas=1 \
  --set higress-core.gateway.resources.limits.cpu=0 \
  --set higress-core.gateway.resources.limits.memory=0 \
  --set higress-core.gateway.resources.requests.cpu=0 \
  --set higress-core.gateway.resources.requests.memory=0 \
  --set higress-core.controller.replicas=1 \
  --set higress-core.controller.resources.requests.cpu=0 \
  --set higress-core.controller.resources.requests.memory=0 \
  --set higress-core.controller.resources.limits.cpu=0 \
  --set higress-core.controller.resources.limits.memory=0 \
  --set higress-core.pilot.replicaCount=1 \
  --set higress-core.pilot.resources.requests.cpu=0 \
  --set higress-core.pilot.resources.requests.memory=0 \
  --set higress-console.replicaCount=0 \
  --set higress-console.resources.requests.cpu=0 \
  --set higress-console.resources.requests.memory=0 \
  --install
