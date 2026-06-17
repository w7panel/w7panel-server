#!/bin/bash

for resource in \
  daemonset/longhorn-iscsi-installation \
  daemonset/longhorn-nfs-installation \
  helmchart/longhorn
do
  if ! kubectl -n kube-system get "$resource" >/dev/null 2>&1; then
    echo "skip missing resource: $resource"
    continue
  fi

  kubectl -n kube-system label "$resource" \
    app.kubernetes.io/managed-by=Helm \
    --overwrite

  kubectl -n kube-system annotate "$resource" \
    meta.helm.sh/release-name=w7panel-longhorn \
    meta.helm.sh/release-namespace=default \
    --overwrite
done
