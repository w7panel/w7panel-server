---
name: w7panel-microapp-local-frontend
description: 在 W7Panel 测试集群中，将已构建的 MicroApp 前端直接复制到面板 Pod 的本地缓存并临时切换 frontendUrl。适用于本地联调或 ZPK 发布暂时不可用时；不用于正式制品发布。
---

# 直传 MicroApp 前端到测试面板

此流程只处理 MicroApp 前端。`w7panel-ui/build.sh` 将主面板前端写入本仓库 `kodata`，`make ko-push` 再将其打进服务端镜像；`pub.sh` 只用 Helm 切换服务端镜像。MicroApp 静态资源由服务端的 `/ui/microapp/:identifie/:version/*path` 路由提供。

1. 确认用户已要求对目标测试集群直传。核对 `kubectl config current-context`、命名空间、面板 Deployment、目标 `microapps.w7panel.w7.com` 的当前 `spec.frontendUrl`。不要假设 Pod 名称或集群 IP 固定。先将该 MicroApp YAML 备份到本地。若仅要求编写说明，不修改集群。
2. 用对应插件项目的构建命令生成前端，确认产物根目录包含 `index.html` 及其引用的静态资源。若只有 `frontend.zip`，先解压到本地临时目录。选一个**未使用**的测试版本目录，保留旧版本以便回退。
3. 查面板 Deployment 的 `microapp-cache` volumeMount 和 volume，确认挂载目录；当前 Chart 挂在 `/data/microapp`，由节点 `hostPath` 提供。确认 `STATIC_IGNORE_LOCAL` 未设为 `true`。将产物复制到 `<挂载目录>/<identifie>/<测试版本>/`，保持 `index.html` 与资源的相对路径。可用 `kubectl cp`，复制后逐项核对文件大小或 SHA-256。不要覆盖旧版本目录。
4. 文件完整后，在新版本目录写入内容恰为 `ok` 的 `.w7panel-cache-complete`。服务端只有检测到该标记才从本地缓存提供文件。然后仅将目标 MicroApp 的 `spec.frontendUrl` 改为 `/ui/microapp/<identifie>/<测试版本>/index.html`；不改 WasmPlugin 或其启停状态。
5. 用 HTTP GET（允许 `index.html` 的 301 跳转）和一个实际 JS/CSS 路径验证返回内容；核对 `X-Frontend-Source: local`、文件内容与构建产物一致，并确认 MicroApp 的 `frontendUrl` 已更新。失败时把 `frontendUrl` 恢复为备份值，保留旧版本文件。记录测试版本、目标集群和回退地址。

此操作不经过 ZPK，也不会生成正式插件版本。缓存使用节点 `hostPath`：Pod 调度到其他节点时文件可能缺失；后续 Helm/ZPK 发布可能覆盖 `frontendUrl`。需要长期保留时，仍应走正式发布流程。
