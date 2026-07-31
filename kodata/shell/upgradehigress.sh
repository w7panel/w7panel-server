#!/bin/bash
set -euo pipefail

VERSION="${1:-${VERSION:-2.2.3}}"
KO_DATA_PATH="${KO_DATA_PATH:-./kodata}"
# CHART_PATH="${KO_DATA_PATH}/charts/higress-${VERSION}.tgz"

# if [ ! -f "$CHART_PATH" ]; then
#   echo "higress chart not found: $CHART_PATH" >&2
#   exit 1
# fi

# The k8s-offline agent removes the K3s HelmChart before this script runs.
# Do not adopt it as a Helm resource: that leaves K3s Addon reconciliation in
# control and can restore an old Higress version after a node restart.
# 2.2.3 版本有问题 1. 会创建默认ingress higress-system 2. ssl证书不生效 返回higress-gateway的证书（ai-proxy.innernal 这ai插件影响了）
helm upgrade w7panel-higress "https://cdn.w7.cc/w7panel/charts/higress/w7panel-higress-${VERSION}.tgz" --install
# helm upgrade w7panel-higress $KO_DATA_PATH/charts/w7panel-higress-2.2.3.tgz --install
