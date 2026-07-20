#!/bin/bash
set -euo pipefail

VERSION="${1:-${VERSION:-2.2.3}}"

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
# 2.2.3 版本有问题 1. 会创建默认ingress higress-system 2. ssl证书不生效 返回higress-gateway的证书（ai-proxy.innernal 这ai插件影响了）
helm upgrade w7panel-higress "https://cdn.w7.cc/w7panel/charts/higress/w7panel-higress-${VERSION}.tgz" --install
