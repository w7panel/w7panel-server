# kodata/charts 说明

该目录保存运行时需要读取的 Helm Chart 源码和离线依赖包，不是主面板 Chart 的维护目录。主面板 Chart 位于仓库根目录 `charts/w7panel/`。

## Chart 源码目录

| 目录 | 用途 |
|------|------|
| `cni/` | CNI、Multus 等集群网络组件模板 |
| `gpustack/` | 应用商店中的 GPUStack 应用 Chart |
| `k8s-offline/` | 离线版面板部署 Chart；与根目录主 Chart 分开维护 |

修改这些目录后应至少执行：

```bash
helm lint kodata/charts/cni
helm lint kodata/charts/gpustack
helm lint kodata/charts/k8s-offline
```

## 离线依赖包

| 文件 | 调用方或用途 |
|------|--------------|
| `cert-manager-v1.19.2.tgz` | `kodata/shell/k3k-agent-upgrade.sh` 安装子集群 cert-manager |
| `higress-2.1.6.tgz` | `kodata/shell/k3k-agent-upgrade.sh` 安装子集群 Higress |
| `higress-2.2.3.tgz` | 保留的 Higress 离线包；当前仓库没有直接调用方，删除前需确认外部部署流程 |
| `victoria-metrics-operator-0.43.0.tgz` | `app/application/console/metricsinstall.go` 安装 VictoriaMetrics Operator |

更新压缩包时，必须同步修改调用方中的文件名和版本，不能只替换本目录文件。

## BootstrapInstallation 管理的内置应用

以下应用不再维护 `kodata/charts/w7panel-*` 本地 Chart，而是由 `kodata/yaml/bootstrap-installations.yaml` 声明并通过 ZPK 首次安装：

- `w7panel-higress`
- `w7panel-cloudnoauth`

默认清单不再预装插件类型应用。

内置声明必须保留 `w7.cc/bootstrap-builtin=true` 标签，升级脚本仅在该标签和 BootstrapInstallation 类型范围内清理已从清单移除的资源，不影响用户自建声明。修改清单后运行：

```bash
go test ./common/service/k8s/bootstrap
```

新增内置应用时优先增加独立 BootstrapInstallation，不要重新增加 `w7panel-*` 本地 Chart 或在 `upgrade.sh` 中直接执行 `helm upgrade`。
