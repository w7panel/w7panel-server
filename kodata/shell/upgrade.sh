#!/bin/sh
set -ex

echo "导入crd"
kubectl apply -f $KO_DATA_PATH/crds --server-side
sh $KO_DATA_PATH/shell/migrate-crd-groups.sh

echo "导入webhook公共CA"
if kubectl get namespace cert-manager >/dev/null 2>&1 && kubectl get crd certificates.cert-manager.io >/dev/null 2>&1 && kubectl get crd clusterissuers.cert-manager.io >/dev/null 2>&1; then
  kubectl apply -f $KO_DATA_PATH/yaml/webhook-ca.yaml
else
  echo "cert-manager未就绪，跳过webhook公共CA"
fi

echo "升级私有DNS"
w7panel privatedns-upgrade

echo "升级用户"
w7panel user-upgrade

echo "升级用户配置"
w7panel w7config-upgrade

echo "升级站点设置"
w7panel site-setting-upgrade

echo "补齐 Ingress 应用分组"
# w7panel ingress-add-group 

echo "导入yaml"
kubectl apply -f $KO_DATA_PATH/yaml/nvidia.yaml

# echo "卸载默认的vm-operator"
# helm list -n w7-system --filter 'vm-operator' | grep -q 'vm-operator' && helm uninstall vm-operator -n w7-system


echo "卸载旧版metrics pod " # 之前helm cleanup.enabled=false 导致无法删除，手动删除
kubectl delete deployment.apps/vmsingle-vm-operator-k8s-offline-metrics-single -n w7-system --ignore-not-found
kubectl delete deployment.apps/vmagent-vm-operator-k8s-offline-metrics-agent -n w7-system --ignore-not-found

# echo "安装k3k"
# helm upgrade --namespace k3k-system --create-namespace k3k $KO_DATA_PATH/charts/k3k-0.3.5.tgz --install --timeout 600s

# kubectl create secret generic k3k-virtual --from-file=$KO_DATA_PATH/yaml/k3k/k3k-virtual.yaml -n k3k-system | echo "已存在k3k-virtual"

# echo "导入k3k 0.3.5 crd"
# kubectl apply -f $KO_DATA_PATH/crds-k3k 

echo "apply longhorn-volumes configmap"
kubectl create -f $KO_DATA_PATH/yaml/longhorn-volumes-config.yaml || echo "已存在longhorn-volumes-config"

echo "创建默认pvc"
# kubectl get pvc default-volume  >/dev/null 2>&1 || kubectl apply -f $KO_DATA_PATH/yaml/default-volume.yaml && kubectl apply -f $KO_DATA_PATH/yaml/default-sc.yaml

# if kubectl get crd settings.longhorn.io &> /dev/null; then
#     echo "CRD settings.longhorn.io 已存在"
#     kubectl create -f $KO_DATA_PATH/yaml/default-sc.yaml || echo "已存在default-sc"
#     kubectl create -f $KO_DATA_PATH/yaml/default-volume-longhorn.yaml || echo "已存在default-volume"
# else
#     echo "CRD settings.longhorn.io 不存在"
#     kubectl create -f $KO_DATA_PATH/yaml/default-volume.yaml || echo "已存在default-volume"
# fi
# install.sh 已使用最新版面板 不需要判断longhorn是否存在
kubectl create -f $KO_DATA_PATH/yaml/default-volume.yaml || echo "已存在default-volume"

# echo "API示例代码"
# kubectl apply -f $KO_DATA_PATH/yaml/code

echo "create权限 不使用apply" 
# kubectl get permission founder >/dev/null 2>&1 || kubectl apply -f $KO_DATA_PATH/yaml/permission/founder.yaml --server-side
kubectl annotate serviceaccount "${SERVICE_ACCOUNT_NAME:-w7panel-offline}" -n "${NAMESPACE:-default}" w7.cc/menu-name- --overwrite || true
kubectl delete permission tech --ignore-not-found
kubectl delete configmap tech -n "${NAMESPACE:-default}" --ignore-not-found
kubectl delete configmap k3k.permission.tech -n "${NAMESPACE:-default}" --ignore-not-found
kubectl delete configmap permission.tech -n "${NAMESPACE:-default}" --ignore-not-found
kubectl create -f $KO_DATA_PATH/yaml/permission || echo "已存在"

# 系统内置权限直接替换，自定义权限不在该目录中
kubectl apply -f $KO_DATA_PATH/yaml/permission

# echo "升级站点管理"
# w7panel sitemanager-upgrade --version=1.0.24 --identifie=w7_php --is-agent=false
# w7panel sitemanager-upgrade --version=1.0.24 --identifie=w7_go --is-agent=false
# w7panel sitemanager-upgrade --version=1.0.24 --identifie=w7_nodejs --is-agent=false
# w7panel sitemanager-upgrade --version=1.0.24 --identifie=w7_python --is-agent=false
# w7panel sitemanager-upgrade --version=1.0.25 --identifie=w7_sitemanager --is-agent=false
echo "longhorn config" 
# longhorn 可能未安装 导致apply 失败 || 不要报错
kubectl apply -f $KO_DATA_PATH/yaml/longhorn/node-down-pod-deletion-policy.yaml || echo "longhorn set node-down-pod-deletion-policy"
kubectl apply -f $KO_DATA_PATH/yaml/longhorn/default-data-locality.yaml || echo "longhorn set default-data-locality"


echo "higress config"
# higress 可能未启动成功 导致crd未创建 job设置重试3次
kubectl apply -f $KO_DATA_PATH/yaml/higress-compressor.yaml --server-side

echo "安装/升级制品版域名和限流插件"
helm upgrade --namespace "${NAMESPACE:-default}" w7panel-pluginwhitedomain $KO_DATA_PATH/charts/w7panel-pluginwhitedomain --install --timeout 600s
helm upgrade --namespace "${NAMESPACE:-default}" w7panel-pluginratelimit $KO_DATA_PATH/charts/w7panel-pluginratelimit --install --timeout 600s
DOMAIN_TARGET_GROUP=w7panel-pluginwhitedomain \
RATE_LIMIT_TARGET_GROUP=w7panel-pluginratelimit \
TARGET_WAIT_SECONDS=600 \
DELETE_LEGACY=true \
sh $KO_DATA_PATH/shell/upgrade-wasm-plugins.sh all



# kubectl create secret generic k3k.addon --from-file=manifests.yaml=$KO_DATA_PATH/yaml/k3k/k3k.addon.yaml --dry-run=client -o yaml | kubectl apply -f - || echo "已存在k3k.addon"

# kubectl apply -f $KO_DATA_PATH/yaml/k3k/virtualclusterpolicy.yaml

# echo "卸载异常面板"
# w7panel uninstall-store-panel

echo "开启Cilium和Hubble Prometheus指标"
if kubectl get crd helmchartconfigs.helm.cattle.io >/dev/null 2>&1 \
  && kubectl get helmchart cilium -n kube-system >/dev/null 2>&1; then
  kubectl apply -f - <<'EOF'
apiVersion: helm.cattle.io/v1
kind: HelmChartConfig
metadata:
  name: cilium
  namespace: kube-system
spec:
  valuesContent: |-
    prometheus:
      enabled: true
    hubble:
      enabled: true
      metrics:
        enableOpenMetrics: true
        enabled:
          - dns:query;ignoreAAAA;sourceContext=pod;destinationContext=pod
          - drop:sourceContext=pod;destinationContext=pod
          - tcp:sourceContext=pod;destinationContext=pod
          - flow:sourceContext=pod;destinationContext=pod
          - icmp:sourceContext=pod;destinationContext=pod
          - http:sourceContext=pod;destinationContext=pod
EOF
else
  echo "未发现K3s Cilium HelmChart，跳过开启Cilium和Hubble Prometheus指标"
fi

echo "新版metrics"
w7panel metrics:upgrade

# echo "升级权限菜单"
# w7panel qx-upgrade

echo "域名解析配置"
w7panel domain-config


echo "longhorn 升级到面板中"
w7panel longhornupgrade

echo "安装/升级cloudnoauth"
helm upgrade --namespace "${NAMESPACE:-default}" w7panel-cloudnoauth $KO_DATA_PATH/charts/w7panel-cloudnoauth --install --timeout 600s
echo "安装/升级higress"
helm upgrade --namespace "${NAMESPACE:-default}" w7panel-higress $KO_DATA_PATH/charts/w7panel-higress --install --timeout 600s


echo "clear completed jobs and pod..."

kubectl get jobs -n default -o json \
  | jq -r '.items[]
    | select(
        any(.status.conditions[]?; (.type == "Complete" or .type == "Failed") and .status == "True")
      )
    | .metadata.name' \
  | xargs -r kubectl delete job -n default

echo "Deleting completed pods..."

kubectl get pods -n default -o json \
  | jq -r '.items[]
    | select(.status.phase == "Succeeded" or .status.phase == "Failed")
    | .metadata.name' \
  | xargs -r kubectl delete pod -n default
