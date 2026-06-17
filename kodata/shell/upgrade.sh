#!/bin/sh
set -ex

echo "导入crd"
kubectl apply -f $KO_DATA_PATH/crds --server-side
sh $KO_DATA_PATH/shell/migrate-crd-groups.sh
# kubeblocks 使用新版配置
# kubectl apply -f $KO_DATA_PATH/crds-kubeblocks --server-side

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

echo "域名白名单插件"
kubectl create -f $KO_DATA_PATH/yaml/w7-white-domain.yaml || echo "已存在wasmplugin w7-white-domain"

kubectl patch wasmplugin w7-white-domain -n higress-system --type=merge -p '{"spec":{"url":"http://w7panel-offline.default.svc:8000/ui/wasm/plugin-domain-1.0.2.wasm"}}'



# echo "API示例代码"
# kubectl apply -f $KO_DATA_PATH/yaml/code

echo "create权限 不使用apply" 
# kubectl get configmap k3k.permission.founder >/dev/null 2>&1 || kubectl apply -f $KO_DATA_PATH/yaml/k3k.permission.founder.yaml --server-side
kubectl create -f $KO_DATA_PATH/yaml/permission || echo "已存在"

# 创始人直接替换
kubectl apply -f $KO_DATA_PATH/yaml/permission/k3k.permission.founder.yaml

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



# kubectl create secret generic k3k.addon --from-file=manifests.yaml=$KO_DATA_PATH/yaml/k3k/k3k.addon.yaml --dry-run=client -o yaml | kubectl apply -f - || echo "已存在k3k.addon"

# kubectl apply -f $KO_DATA_PATH/yaml/k3k/virtualclusterpolicy.yaml

echo "卸载异常面板"
w7panel uninstall-store-panel

echo "新版metrics"
w7panel metrics:upgrade

# echo "升级权限菜单" # cvm版本 去掉
# w7panel qx-upgrade

echo "域名解析配置"
w7panel domain-config
kubectl get jobs -n default -o json \
  | jq -r '.items[]
    | select(
        any(.status.conditions[]?; (.type == "Complete" or .type == "Failed") and .status == "True")
      )
    | .metadata.name' \
  | xargs -r kubectl delete job -n default

echo "longhorn 升级到面板中"
w7panel longhornupgrade

echo "限流配置"
# apply -f 会覆盖原有配置 所以使用create 
kubectl create -f $KO_DATA_PATH/yaml/longhorn/cluster-key-rate-limit.yaml || echo "已存在longhorn cluster-key-rate-limit"

# fix 旧版安装longhorn 导致helm 标签丢失
echo "fix longhorn helm labels"
kubectl delete appgroups.w7panel.w7.com/longhorn --wait=false --ignore-not-found
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
