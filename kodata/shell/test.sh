#!/bin/bash

if ! kubectl create -f - <<'EOF'; then
kind: ConfigMap
apiVersion: v1
metadata:
  name: longhorn-volumes-config
data:
  customs: default-volume
  default: default-volume
EOF
  echo "longhorn-volumes-config 已存在"
fi

echo "配置k3s.config configmap..."
kubectl -n kube-system create configmap k3s.config --from-literal=k3s.mode=4 --dry-run=client -o yaml | kubectl create -f - || echo "k3s.config 已更新"