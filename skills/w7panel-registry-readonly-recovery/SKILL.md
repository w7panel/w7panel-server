---
name: w7panel-registry-readonly-recovery
description: 排查并恢复 W7Panel 镜像仓库 /var/lib/registry 只读，尤其是 Longhorn PVC 故障后 ext4 保护性只读；用于 218 集群同类故障，先核对实际对象再操作。
---

# Registry 的 Longhorn 卷只读恢复

适用：Registry 上传报 `read-only file system`，而 Deployment、PVC 或 Longhorn 卷仍显示正常。先确认当前 kubectl context、namespace、Deployment、PVC 和节点；下文的 218 案例是历史证据，不代表这些名称永远不变。用户只要求排查时停在诊断结论；用户要求修复时再执行会中断服务的步骤。

## 已验证案例：218 集群，2026-11-12

- `default/w7-zpkv2-registry` 的 `/var/lib/registry` 来自 PVC `w7-zpkv2-registry`，PV/Longhorn 卷为 `pvc-962d86d3-7196-405d-b3c0-3d67e8a902c5`，位于 `server1`，ext4、RWO、单副本。
- Registry 的 PUT/POST 返回 500，日志中的实际错误是 `filesystem: ... read-only file system`。Pod `Running/Ready`、PVC `Bound`、Longhorn 卷 `attached/healthy`，Deployment 的挂载声明也为 `rw`；这些状态都不能证明文件系统可写。
- Pod 内创建临时文件同样报只读。`/proc/self/mounts` 曾显示 `rw,relatime,emergency_ro`。卷使用率约 39%，inode 使用率约 5%，排除了空间和 inode 耗尽。
- Longhorn 事件显示卷自动恢复并请求重新挂载。节点内核日志显示该卷设备 `sdt` 出现写入 I/O 错误、ext4 journal 中止和文件系统保护性只读；`sdt` 通过 Pod 的 `/proc/self/mountinfo` 中 `65:48` 设备号核对。内核日志还出现其他卷的 I/O 错误，**底层 I/O 故障的最初诱因未查明**。单副本增加再次故障时的数据风险。
- 修复时创建并确认 Longhorn 快照 `registry-before-fsck-20261112`，暂停 Deployment，手动把卸载后的 Longhorn 卷附加到节点，对未挂载的块设备运行 `e2fsck -fp` 恢复 journal，再次 `e2fsck -fn` 无错误；撤销手动附加并恢复 Deployment。Pod 内真实写入和清理成功，挂载不再有 `emergency_ro`，卷为 `attached/healthy`。

## 排查

1. 从 Deployment 的 `volumeMounts` 找到 `/var/lib/registry` 对应的 PVC，再从 PV 的 `spec.csi.volumeHandle` 找到 Longhorn 卷。核对 Pod 所在节点和 Longhorn 的副本数；不要直接套用案例中的 UID。
2. 查 Registry 写入请求日志、Pod 事件、Longhorn 卷及事件。用 Pod 内 `/proc/self/mounts`、`df -h`、`df -i` 和一个随即删除的临时文件确认实际可写性。不要只看 `Ready`、`Bound` 或 Longhorn `healthy`。
3. 查节点内核日志中的目标设备 I/O 与 ext4 错误。案例中通过节点上的 Longhorn manager 容器执行 `dmesg -T`；先用 mountinfo 的设备号对应到内核设备名，避免把其他卷的错误归到 Registry。

## 恢复

1. 记录当前 Deployment 副本数及卷状态。创建 Longhorn 快照并等待 `status.readyToUse=true`，确认有恢复点；单副本时尤其重要。先排除持续的底层 I/O 故障，若卷仍不稳定，停止文件系统修复并处理存储故障。
2. 将 Registry Deployment 缩到 0，等待所有使用该 PVC 的 Pod 退出。确认 Kubernetes `VolumeAttachment` 已移除、Longhorn 卷已 detached，节点挂载表不再包含该卷。不要对仍挂载的 ext4 运行写入式 fsck。
3. 给 Longhorn `volumeattachment.longhorn.io/<卷名>` 临时添加 `longhorn-api` 类型、指向原节点的维护 ticket，等待卷 attached 和 ticket satisfied。确认节点 `/dev/longhorn/<卷名>` 存在且没有挂载。案例通过 Longhorn manager 的 `/host/dev/longhorn/<卷名>` 访问宿主机设备。
4. 先运行 `e2fsck -fn <设备>` 只读检查，再运行 `e2fsck -fp <设备>` 修复并恢复 journal。退出码 1 表示已修复；若要求人工处理、出现 I/O 错误或退出码大于 1，停止并保留快照，不要盲目 `-y`。再次运行 `e2fsck -fn`，应为退出码 0。
5. 只删除本次添加的维护 ticket，等待卷 detached，再把 Deployment 恢复到原副本数。确认 Pod Ready、卷 attached/healthy，`/var/lib/registry` 挂载没有 `emergency_ro`，并在该目录做创建、读取、删除临时文件的实际写入测试。检查新 Pod 日志有无只读或 I/O 错误；能做镜像推送时再验证一次完整推送。

不要删除 PVC、PV、Longhorn 卷或原有快照。Registry Pod 重启本身不能修复已经中止的 ext4 journal。若多个卷同时报 I/O 错误，恢复 Registry 后仍需追查节点和 Longhorn 的共同故障源，避免再次只读。
