## Adopt existing resources into this Helm release

If `helm upgrade w7panel-longhorn ./w7panel-longhorn` fails because one of
these resources already exists without Helm ownership metadata, patch the live
resource before rerunning the upgrade:

```
for resource in \
  daemonset/longhorn-iscsi-installation \
  daemonset/longhorn-nfs-installation \
  helmchart/longhorn
do
  kubectl -n kube-system label "$resource" \
    app.kubernetes.io/managed-by=Helm \
    --overwrite

  kubectl -n kube-system annotate "$resource" \
    meta.helm.sh/release-name=w7panel-longhorn \
    meta.helm.sh/release-namespace=default \
    --overwrite
done
```

Then rerun:

```
helm upgrade w7panel-longhorn ./w7panel-longhorn
```

## Longhorn uninstall confirmation

```
kubectl -n longhorn-system patch -p '{"value": "true"}' --type=merge lhs deleting-confirmation-flag

```
# Longhorn
复查后发现 live 对象还是 1.10.1。原因更具体了：Helm release 记录已经是 1.11.1，后续普通 helm upgrade 做三方 diff 时认为 desired 没变，无法修复这个 live drift。现在需要
  强制替换或直接 patch live HelmChart。

# 地址
https://charts.longhorn.io/index.yaml

https://github.com/longhorn/charts/releases/download/longhorn-1.12.0/longhorn-1.12.0.tgz