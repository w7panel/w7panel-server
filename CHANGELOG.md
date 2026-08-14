# CHANGELOG

## 2026-08-14

- Pod 流量聚合新增持久化 `upstream_pod_name` 维度；工作负载筛选后的 Pod 列表优先显示并搜索入库时的 Pod 名称，旧日志仍兼容当前 IP 反查。
- 影响模块：流量 Pod 查询与 Apps 明细抽屉。
- 验证：待执行流量服务定向测试。

## 2026-08-14

- 新增 `/panel-api/v1/traffic/apps`，按日志入库时保存的 Kubernetes 工作负载类型、名称和命名空间聚合流量，并支持以工作负载筛选汇总、趋势、域名和热点 URL；旧的仅 Pod/IP 日志不会进入 Apps 排行，避免 Pod 重建后的错误归属。
- 影响模块：流量查询 API 与 normal 角色权限。
- 验证：待执行流量服务定向测试。

## 2026-08-14

- 新增根目录 `Makefile`，参考 CNB 的 ko 参数支持将单平台 W7Panel 镜像构建并加载到本机 Docker。
- 影响模块：本地镜像构建流程。
- 验证：安装 `ko v0.19.1` 后运行 `make image`，成功构建并加载 `w7panel:dev-v1`（linux/amd64）到本机 Docker。

## 2026-08-14

- 新增 `make docker-run`，支持挂载宿主 kubeconfig、配置 OIDC Issuer 和 HTTP 服务端口并在后台启动本地 W7Panel 容器。
- 影响模块：本地容器运行流程。
- 验证：通过宿主及容器 18000 端口启动 `w7panel-local`，确认 kubeconfig 只读挂载、OIDC Issuer 与 `W7PANEL_HTTP_SERVER_PORT` 使用 18000，且 Discovery 接口返回 HTTP 200。

## 2026-08-14

- 本地 `make image` 调用相邻 `w7panel-ui/build.sh` 完成前端打包和 `kodata` 静态资源准备，确保 ko 镜像包含 `index.html` 与 `panel.html`。
- 影响模块：本地前端及镜像构建流程。
- 验证：构建镜像并启动容器，确认面板根路径不再因缺少前端入口文件返回 HTTP 500。

## 2026-08-13

- 修复 Longhorn PVC 扩容在“正在重启 Pod”阶段卡住：Deployment 重建后的 Pod 名称会变化，控制器此前只按旧名称检查，导致新 Pod 已 Ready 仍无法完成。现按原 Controller UID 识别新名称的 Ready 替代 Pod；旧任务缺少该 UID 时回退检查 Longhorn 当前挂载的 Ready 工作负载，StatefulSet 同名 Pod 仍沿用原检查。
- 影响模块：Longhorn PVC 扩容控制器与回归测试。
- 验证：新增 Deployment 更名替代 Pod 的就绪判定测试。

## 2026-08-10

- 为普通用户补充流量监测微应用所需的只读接口权限：健康检查、汇总、趋势、Pod、域名和 URL 查询；避免 normal 角色打开流量监测时被权限拦截。
- 影响模块：`kodata/yaml/permission/normal.yaml`。
- 验证：已核对 `w7panel-traffic` 的实际请求与后端 `/panel-api/v1/traffic/*` 路由定义，权限范围不包含仅 founder 使用的命名空间接口。

## 2026-08-10

- 限制 normal 用户的流量监测查询始终使用宿主集群的 `k3k-{用户名}` 命名空间，忽略请求中的 `namespace` 参数，防止跨 K3K 用户读取 Higress 流量数据。
- 影响模块：流量查询参数解析与控制器测试。
- 验证：新增测试断言 normal 用户 `minghu` 的查询命名空间固定为 `k3k-minghu`。

## 2026-08-05

- 修复顶部微应用角色数量判断：只统计面板支持的 `founder`、`super`、`normal` Binding，`zpk-market` 等功能菜单分组不再把单角色应用提升到顶部菜单。
- 影响模块：面板角色定义、MicroApp 顶部列表接口。
- 验证：`go test ./common/service/k8s/microapp -run TestPanelRoleBindingCount -count=1` 与 `go test ./common/service/k8s/permission -run TestIsPanelRole -count=1` 通过。

## 2026-08-05

- 新增项目开发规则：此后每次修改代码、配置、测试或文档，都必须追加更新本文件。
- 影响模块：项目开发与提交流程。
- 验证：已确认 `AGENTS.md` 包含追加写入和提交前检查要求。

- 修复 Longhorn PVC 扩容完成后删除临时 ticket 导致卷未重新绑定的问题；恢复原 CSI attachment ticket，并等待关联 Pod 重启就绪后再标记成功。
- 验证：定向 Go 测试和后端构建通过，`data-test-postgresql-0` 完成在线扩容并恢复 CSI 绑定，关联 Pod 重建后正常运行。
## 2026-08-05

- 统一 MicroApp 菜单权限过滤：founder 展示全部 Binding 菜单，其他角色沿用现有角色过滤规则，不在面板中硬编码具体菜单分组名称。
- 影响模块：MicroApp 列表、详情及根 MicroApp 同步过滤。
- 验证：`go test ./common/service/k8s/microapp` 通过。

## 2026-08-05

- 删除 AppGroup 外部服务字段及其 ZPK 响应映射、安装转换、生成客户端和测试代码，服务中心入口统一由 MicroApp Binding 提供。
- 影响模块：AppGroup CRD、ZPK 安装与 ManifestPackage、Kubernetes 生成代码、后端文档。
- 验证：`go build ./...`、AppGroup 生成客户端相关包测试及 `TestSyncAppGroupZpkURL` 通过。

## 2026-08-06

- 修复 AppGroup 静态资源解压路径穿越、父子应用删除卡住及 MicroApp 卸载残留问题。
- 影响模块：`common/service/k8s/appgroup`。
- 验证：运行 AppGroup 定向 Go 测试。

## 2026-08-07

- 控制台 OAuth 注册新 User 时初始化 `spec.cloud.userInfo`，确保 User CRD 保留云端用户配置。
- 影响模块：控制台认证、User CRD 序列化。
- 验证：运行 User 服务定向 Go 测试。

## 2026-08-07

- 修复 `w7config-upgrade` 重复执行时旧 Secret 覆盖 User CRD 最新 `spec.cloud` 配置的问题。
- 影响模块：w7-config 到 User CRD 迁移。
- 验证：运行配置服务定向 Go 测试。

## 2026-08-07

- 控制台每次 OAuth 登录同步 User CRD 的 `spec.cloud` 用户信息及兼容字段，并保留 User 元数据。
- 影响模块：控制台认证、User CRD。
- 验证：运行 User 服务定向 Go 测试。
