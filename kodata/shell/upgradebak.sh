#!/bin/sh
set -ex

echo "导入crd"
kubectl apply -f $KO_DATA_PATH/crds --server-side
# kubeblocks 使用新版配置
# kubectl apply -f $KO_DATA_PATH/crds-kubeblocks --server-side

echo "导入yaml"
kubectl apply -f $KO_DATA_PATH/yaml/nvidia.yaml 

# echo "卸载默认的vm-operator"
# helm list -n w7-system --filter 'vm-operator' | grep -q 'vm-operator' && helm uninstall vm-operator -n w7-system


echo "卸载旧版metrics pod " # 之前helm cleanup.enabled=false 导致无法删除，手动删除
kubectl delete deployment.apps/vmsingle-vm-operator-k8s-offline-metrics-single -n w7-system --ignore-not-found
kubectl delete deployment.apps/vmagent-vm-operator-k8s-offline-metrics-agent -n w7-system --ignore-not-found

adopt_helm_crd() {
    crd_name="$1"

    if ! kubectl get crd "$crd_name" >/dev/null 2>&1; then
        echo "crd $crd_name 不存在，交给 helm 创建"
        return 0
    fi

    kubectl label crd "$crd_name" app.kubernetes.io/managed-by=Helm --overwrite
    kubectl annotate crd "$crd_name" meta.helm.sh/release-name=k3k --overwrite
    kubectl annotate crd "$crd_name" meta.helm.sh/release-namespace=k3k-system --overwrite
}

inject_legacy_crd_version() {
    crd_name="$1"
    crd_file="$2"
    legacy_version="$3"

    if ! kubectl get crd "$crd_name" >/dev/null 2>&1; then
        return 0
    fi

    stored_versions="$(kubectl get crd "$crd_name" -o jsonpath='{.status.storedVersions}' 2>/dev/null || true)"
    if ! echo "$stored_versions" | grep -q "$legacy_version"; then
        return 0
    fi

    if grep -q "name: $legacy_version" "$crd_file"; then
        echo "$crd_name 已包含 $legacy_version，无需补丁"
        return 0
    fi

    echo "$crd_name 检测到 storedVersions 包含 $legacy_version，给 chart 注入兼容版本"
    python3 - "$crd_file" "$legacy_version" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
legacy = sys.argv[2]
lines = path.read_text().splitlines()

versions_idx = next(i for i, line in enumerate(lines) if line.strip() == "versions:")
start = versions_idx + 1
end = len(lines)
for i in range(start + 1, len(lines)):
    if lines[i].startswith("  ") and not lines[i].startswith("    ") and not lines[i].startswith("  -"):
        end = i
        break

block = lines[start:end]
patched = []
name_replaced = False
storage_replaced = False
for line in block:
    if not name_replaced and line.strip() == "name: v1beta1":
        indent = line[:len(line) - len(line.lstrip())]
        patched.append(f"{indent}name: {legacy}")
        name_replaced = True
        continue
    if not storage_replaced and line.strip() == "storage: true":
        indent = line[:len(line) - len(line.lstrip())]
        patched.append(f"{indent}storage: false")
        storage_replaced = True
        continue
    patched.append(line)

if not name_replaced:
    raise SystemExit(f"unable to find v1beta1 version block in {path}")

updated = lines[:start] + patched + block + lines[end:]
path.write_text("\n".join(updated) + "\n")
PY
}

echo "安装k3k"
K3K_TMP_DIR="$(mktemp -d /tmp/k3k-chart.XXXXXX)"
trap 'rm -rf "$K3K_TMP_DIR"' EXIT
tar -xzf "$KO_DATA_PATH/charts/k3k-1.0.2.tgz" -C "$K3K_TMP_DIR"

inject_legacy_crd_version clusters.k3k.io "$K3K_TMP_DIR/k3k/templates/crds/k3k.io_clusters.yaml" v1alpha1
inject_legacy_crd_version virtualclusterpolicies.k3k.io "$K3K_TMP_DIR/k3k/templates/crds/k3k.io_virtualclusterpolicies.yaml" v1alpha1

adopt_helm_crd clusters.k3k.io
adopt_helm_crd virtualclusterpolicies.k3k.io

helm upgrade --namespace k3k-system --create-namespace k3k "$K3K_TMP_DIR/k3k" --install --timeout 600s

kubectl create secret generic k3k-virtual --from-file=$KO_DATA_PATH/yaml/k3k/k3k-virtual.yaml -n k3k-system | echo "已存在k3k-virtual"

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



echo "API示例代码"
kubectl apply -f $KO_DATA_PATH/yaml/code

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

echo "k3k addons"

kubectl create secret generic k3k.addon --from-file=manifests.yaml=$KO_DATA_PATH/yaml/k3k/k3k.addon.yaml --dry-run=client -o yaml | kubectl apply -f - || echo "已存在k3k.addon"


echo "卸载异常面板"
w7panel uninstall-store-panel

echo "新版metrics  "
w7panel metrics:upgrade

echo "升级权限菜单"
w7panel qx-upgrade

echo "域名解析配置"
w7panel domain-config
