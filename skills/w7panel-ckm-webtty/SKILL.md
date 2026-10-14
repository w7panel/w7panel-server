---
name: w7panel-ckm-webtty
description: 排查或修改 CKM 详情页 WebTTY/WebShell 经 W7Panel MicroApp 代理时的 401、403、断连或 WebSocket 升级问题；涉及 w7panel-server、w7panel-ckm 或其前端终端组件时使用。
---

# CKM WebTTY 端到端排查

WebTTY 链路为：浏览器 → `/panel-api/v1/microapp/:name/proxy/...` → `w7panel-server` MicroApp reverse proxy → `w7panel-ckm` `/ckm-api/v1/ckms/:namespace/:name/pods/:pod/exec` → 根集群 Pod exec。

## 关键约束

- 浏览器 WebSocket 不能传自定义认证 Header。前端在 `Sec-WebSocket-Protocol` 中发送 `ckm-terminal`、`w7panel-bearer.<panel JWT>` 与 `bearer.<CKM OIDC token>`；不要把 token 放进 URL。
- `w7panel-server/common/middleware/panelauth.go` 从 `w7panel-bearer.` 认证外层面板请求；CKM 从 `bearer.` 验证 OIDC access token。两者职责不同，不能任意删除其中一个。
- MicroApp reverse proxy 默认把上游 `Host` 改成内部 Service。对 WebSocket 必须保留原始客户端 Host，否则浏览器 Origin 与内部 Host 不一致，CKM Gorilla upgrader 的默认同源校验会在升级前返回 403。
- CKM 的旧 OIDC issuer 可能只把角色放进 ID token；CKM 在内存中关联 access token 和 ID token 角色。CKM 重启会清空该关联，所以 WebTTY 建立前需要重新交换 Wujie 的 OIDC code，不可只依据 access token 是否过期。
- 终端授权必须同时保留 CKM 所有权（管理员或目标 namespace 所有者）、Pod 属于 CKM，以及容器属于 Pod 的检查。不要为修复连接而放宽这些边界。

## 定位顺序

1. 在浏览器 Network 中确认 WebSocket URL 经 MicroApp proxy，而非直接 Kubernetes proxy；截图确认弹窗实际状态。
2. 查 `w7panel-ckm` 日志中的 `/exec`：
   - `401 invalid access token` 或 `role claims are unavailable`：先重新交换 OIDC code；
   - `403` 且没有 CKM 归属拒绝日志：优先检查 server proxy 改写 Host 后的 Origin 校验；
   - 已升级后立即断开：检查 Kubernetes `RunExec` 和选定容器。
3. 检查 server 的 `ProxyMicroApp` 和 `helper.ProxyUrl` 是否保留 Upgrade、Connection、Sec-WebSocket-Protocol，并只在 WebSocket 路径保留客户端 Host。
4. 检查 CKM 的 `execCkmPod`、`loadCkmForRuntimeDetail`、`loadCkmPod`；普通用户只能访问自己 `k3k-<username>` namespace 的 CKM，founder/admin 可跨 namespace。

## 验证

- 后端改动至少运行目标 Go 测试；CKM 前端改动运行 `make build-ui`。
- 需要真实环境验证时，先构建并更新 server 与 CKM 镜像，再在已登录共享浏览器打开 CKM 详情页并点击 WebTTY。
- 成功证据是终端出现实际 Shell 提示符（如 `~ #`）；Gin 对升级连接可能按最终完成状态记录为 `200`，不应仅凭日志不是 `101` 判定失败。
- 不在 Skill 或日志中记录账号、JWT、OIDC code、kubeconfig 或 Secret。
