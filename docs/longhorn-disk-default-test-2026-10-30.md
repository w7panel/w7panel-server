# Longhorn `disk-default` 现场测试（2026-10-30）

## 目标与环境

- 目标一：首次安装 Longhorn 时创建的 `disk-default` 磁盘可以删除，删除后不会自动重建。
- 目标二：`disk-default` 只在首次安装 Longhorn 时为当时已有的控制平面节点创建；后续新增节点不会自动创建。
- 环境：本机卸载原 `k3s-agent` 后，安装为独立 K3s server `server1`（K3s `v1.36.5+k3s1`，Debian 13），再安装 Longhorn `v1.11.1`。
- 安装命令使用 `curl -sfL https://cdn.w7.cc/w7panel/install.sh | sh -s - k3s`。原写法 `sh - k3s` 会把 `k3s` 当作脚本文件，无法执行。

现场调用当前仓库的 `longhorn.PrepareDefaultDiskInstall`、应用 `kodata/yaml/longhorn/longhornfull.yaml`，并调用 `longhorn.OnStart` 检查清理逻辑。调用程序放在 `/tmp`，没有提交到仓库。由于工作区原有未跟踪的 `common/service/procpath/nsenter.go` 引用缺失的 `InnerFilePath`，完整控制器无法编译，本次没有通过 HTTP `/panel-api/v1/longhorn/install` 接口发起安装。

## 结果

| 场景 | 操作与观测 | 结论 |
| --- | --- | --- |
| 首次安装 | `PrepareDefaultDiskInstall` 仅标记当时已有的 `server1`；Longhorn 创建 `/var/lib/longhorn` 下的 `disk-default`，磁盘标签为 `disk-default`。调用 `OnStart` 后，节点上的 `node.longhorn.io/create-default-disk` 标签、`node.longhorn.io/default-disks-config` 和 `w7panel.w7.com/default-disk-install-pending` 注解均被清除。 | 通过 |
| 实际分配与回收 | 使用 `storageClassName: disk-default` 创建 64Mi PVC，状态变为 `Bound`；删除 PVC 后，对应 PV 和 Longhorn 卷均被回收。 | 通过 |
| 删除默认磁盘 | 删除前确认 Longhorn 没有 Volume 或 Replica。直接删除磁盘被 Longhorn 拒绝，提示先禁用调度；将 `spec.disks.disk-default.allowScheduling` 设为 `false` 后删除成功。再次调用 `OnStart`，磁盘仍为空，`disk-default` StorageClass 也被清理。 | 通过；需遵循 Longhorn 的先禁用再删除流程 |
| 安装后新增节点 | 安装后创建临时 Kubernetes Node `w7panel-new-node-check`，设置控制平面标签，等待后再调用 `OnStart`。该节点没有获得建盘标签或注解，`server1` 的磁盘也没有重建。测试 Node 随后删除。 | 模拟检查通过；尚未在真实新加入的 K3s 节点上验证 |

新增节点测试使用的是 API 创建的 Node 对象，没有 kubelet，因此 Longhorn 没有为它生成 Node CR。该结果验证了当前项目不会在此场景重新写入建盘触发配置，**不能代替真实节点加入集群后的端到端验证**。

## 测试后状态与限制

- 独立 K3s server 和面板正常运行；Longhorn 组件均为 Running。
- `server1` 的 `disk-default` 已按删除测试移除，测试 PVC、PV、Longhorn 卷及临时 Node 均已清理。需要使用该磁盘时，须手动重新配置。
- `disk-default` 存在期间可用于 PVC 分配，但并非集群默认 StorageClass；默认项是 `local-path`。
- 定向单元测试 `make test TEST_PACKAGES=./common/service/k8s/longhorn` 通过。
- 安装期间曾因旧 agent 遗留的 `OLD_CILIUM_*` iptables 规则和不可达的 Pod DNS 导致 Helm 任务失败；清理旧规则、配置可用 DNS 后安装继续。Longhorn 镜像拉取时出现摘要不匹配，改用可用镜像源后组件启动。上述问题属于本次环境准备，不改变 `disk-default` 测试结论。
