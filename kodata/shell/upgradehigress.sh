#!/bin/bash
set -euo pipefail

VERSION="${1:-${VERSION:-2.2.3}}"
KO_DATA_PATH="${KO_DATA_PATH:-./kodata}"
# CHART_PATH="${KO_DATA_PATH}/charts/higress-${VERSION}.tgz"

# if [ ! -f "$CHART_PATH" ]; then
#   echo "higress chart not found: $CHART_PATH" >&2
#   exit 1
# fi

for resource in \
  helmchart/higress
do
  if ! kubectl -n kube-system get "$resource" >/dev/null 2>&1; then
    echo "skip missing resource: $resource"
    continue
  fi

  kubectl -n kube-system label "$resource" \
    app.kubernetes.io/managed-by=Helm \
    --overwrite

  kubectl -n kube-system annotate "$resource" \
    meta.helm.sh/release-name=w7panel-higress \
    meta.helm.sh/release-namespace=default \
    --overwrite
done

helm upgrade w7panel-higress "https://cdn.w7.cc/w7panel/charts/higress/w7panel-higress-${VERSION}.tgz" --install
# helm upgrade w7panel-higress $KO_DATA_PATH/charts/w7panel-higress-2.2.3.tgz --install
