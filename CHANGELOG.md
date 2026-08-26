# CHANGELOG

## 2026-08-26

- K3k Agent 升级脚本新增 `default-volume` PVC 检查，不存在时才创建，避免重复创建产生无效错误。
- 影响模块：K3k Agent 升级流程。
- 验证：脚本静态检查与 `git diff --check`。

## 2026-08-26

- 普通用户权限新增 `GET /panel-api/v1/zpk/domain-parse`，用于安装页读取主集群域名解析配置；其他 ZPK 接口仍不开放。
- 影响模块：普通用户权限配置、权限匹配测试。
- 验证：定向权限测试与 `git diff --check`。

## 2026-08-17

- 应用详情页打开已安装制品时，回源校验制品试用权益；试用到期返回 `ZPK_TRIAL_EXPIRED`，面板提示续费。
## 2026-08-25

- 修复 Console ZPK Helm 应用卸载：非 `default` 命名空间的删除中 AppGroup 会进入清理协调；Helm release 卸载失败时保留 finalizer 并持续限速重试，避免应用记录删除后遗留 Helm 资源。
- 影响模块：AppGroup 删除控制器与事件队列。
- 验证：新增 AppGroup 删除协调定向测试通过；完整 `go test ./common/service/k8s/appgroup -count=1` 在当前 30 秒执行窗口内未完成。

## 2026-08-24

- 完善 BuildImage CRD 任务生命周期：Job 默认失败后重试 3 次，成功或最终失败后保留 CRD 并由 TTL 在 5 分钟后自动清理 Job，避免任务被清理后重复创建。
- 扩展构建状态记录重试次数、最大重试次数和完成时间。
- 修正 BuildImage 重试进度，排除首次失败 Pod，避免最终失败显示为 `4/3`。

## 2026-08-21

- 支付订单通知由 Server 接收后通过集群内 CKM Service 转发，新增 `CKM_ORDER_NOTIFY_URL` 配置覆盖默认地址。
- 影响模块：K3k 订单回调。
- 验证：`go test ./app/k3k/http/controller -count=1` 通过（该包暂无测试文件）。

## 2026-08-20

- 调整 Longhorn 分区扩容成功判定：关联 Pod 删除请求执行成功后不再等待新 Pod Ready，Pod 重启超时不会再导致分区扩容任务失败；仍保留扩容容量与存储附件恢复校验。
- 影响模块：Longhorn PVC 扩容控制器与回归测试。
- 验证：`GOTMPDIR=/tmp/w7panel-go-tmp GOCACHE=/tmp/w7panel-go-cache go test ./common/service/k8s/longhorn -run 'TestPVCResize' -count=1` 通过。

## 2026-08-19

- Apps 和 Pod 流量聚合增加 `workload_title` 展示字段；缺失该字段的旧日志仍回退使用原始 workload 名称。
- 影响模块：流量查询 API。
- 验证：待执行流量服务定向测试。

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
## 2026-08-05（BootstrapInstallation）

- 移除无实际协调作用的 `BootstrapInstallation.spec.revision`，Operation ID 改为基于资源 UID 稳定生成。
- 为内置 BootstrapInstallation 增加专用标签，并在升级脚本中通过标签和资源类型 allowlist 安全清理已从清单移除的声明。
- 影响模块：BootstrapInstallation API、CRD、内置清单、升级脚本、测试与开发文档。
- 验证：API/CRD/内置清单回归测试、Bootstrap 核心测试（排除工作区中既有的未完成 Token 测试）、升级脚本语法检查及两套 Helm Chart lint 均通过；完整 Bootstrap 包测试仍被既有 `clusterTokenFromRESTConfig` 缺失阻断。
- 修复 Longhorn PVC 扩容完成后删除临时 ticket 导致卷未重新绑定的问题；恢复原 CSI attachment ticket，并等待关联 Pod 重启就绪后再标记成功。
- 验证：定向 Go 测试和后端构建通过，`data-test-postgresql-0` 完成在线扩容并恢复 CSI 绑定，关联 Pod 重建后正常运行。

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

## 2026-08-11（BootstrapInstallation 更新）

- 使用 `metadata.generation` 与新增的 `status.observedGeneration` 协调更新：Generation 变化时直接按 `spec.artifact.version` 请求固定版本或 `latest`，不再读取 AppGroup 版本比较。
- 新增 `spec.revision` 手动触发器，可在制品版本不变时通过修改 revision 再次更新；更新过程复用安装并发槽、超时和重试策略，仅允许更新当前 BootstrapInstallation UID 所有的 AppGroup。
- 影响模块：BootstrapInstallation Controller、ZPK 执行器、API/CRD、测试和使用文档。
- 验证：Bootstrap Controller 定向测试通过，覆盖 Generation 去重、Generation 变化直接更新、`latest` 更新及完成状态收口场景。

## 2026-08-11（BootstrapInstallation 版本检测更新）

- 将更新触发方式调整为周期查询 ZPK `check_upgrade` 接口并比较 AppGroup 当前版本，移除 `spec.revision`；空版本或 `latest` 自动跟踪更高版本，固定版本在不一致时执行更新或回退。
- Ready 状态每 10 分钟检查一次，检测失败 1 分钟后重试；更新仍复用并发槽、超时和重试策略，并按检测到的目标版本区分执行与重试周期。
- 影响模块：BootstrapInstallation Controller、ZPK 执行器、API/CRD、测试和使用文档。
- 验证：BootstrapInstallation API/CRD 测试通过；Controller 定向测试覆盖周期检测、同版本去重、固定版本与最新版更新，完整包测试仍受工作区既有未完成的 `clusterTokenFromRESTConfig` 测试阻断。

## 2026-08-11（BootstrapInstallation 状态精简）

- 移除 `status.observedGeneration`、`status.operationGeneration` 和 `status.operationVersion`，更新完成判断改为使用执行阶段及 AppGroup 实际状态，ZPK 执行 ID 直接结合制品实际版本生成。
- 更新重试达到上限后按版本检测周期冷却，再开启新的重试周期，无需在资源状态中持久化 Generation 或目标版本。
- 影响模块：BootstrapInstallation Controller、API/CRD、测试和使用文档。
- 验证：运行 Bootstrap Controller 与 BootstrapInstallation API/CRD 定向测试。

## 2026-08-11（BootstrapInstallation 更新检查触发）

- 取消 Ready 状态的定期版本轮询；仅在资源创建、spec 变更、Controller 启动扫描等实际协调事件发生时查询 ZPK 版本，无更新后不再主动入队。
- ZPK 查询失败仍保留 1 分钟错误重试，已达到更新重试上限的资源不安排周期任务。
- 影响模块：BootstrapInstallation Controller、测试和使用文档。
- 验证：运行 Bootstrap Controller 定向测试，覆盖协调时检查和无更新不重排队。

## 2026-08-11（BootstrapInstallation ZPK 查询参数）

- 根据 ZPK 服务端当前实现移除无效的 `check_upgrade` 请求参数，版本查询仅使用实际生效的 `is_upgrade=1` 和 `cur_version`，并继续比较接口返回版本。
- 影响模块：BootstrapInstallation ZPK 执行器、测试和使用文档。
- 验证：ZPK HTTP 模拟测试覆盖无 `check_upgrade` 参数及有效升级参数。

## 2026-08-11（BootstrapInstallation 固定版本预检）

- 固定 `spec.artifact.version` 在请求 ZPK 前先与 AppGroup 已安装版本比较；版本一致时直接结束，只有不一致时才查询并加载指定制品。
- 空版本和 `latest` 仍请求 ZPK 以获取远端可用版本。
- 影响模块：BootstrapInstallation ZPK 执行器、测试和使用文档。
- 验证：HTTP 模拟测试覆盖语义版本相同场景不产生 ZPK 请求。

## 2026-08-11（BootstrapInstallation 版本职责拆分）

- 将固定版本是否需要查询的判断从 ZPK installer 移至通用版本模型，并由 Controller 在调用 installer 前决策；installer 仅负责 ZPK 请求、解析与执行。
- 影响模块：BootstrapInstallation Controller、版本模型、ZPK 执行器和测试。
- 验证：通用版本策略表驱动测试及 Controller 同版本不调用 installer 测试通过。

## 2026-08-11（BootstrapInstallation 候选制品解析）

- 进一步将远端候选版本比较移出 ZPK installer：installer 只解析并返回候选制品，Controller 使用通用版本模型决定升级、回退或忽略。
- 影响模块：BootstrapInstallation Controller、版本模型、ZPK 执行器和测试。
- 验证：候选制品 HTTP 测试和升级决策表驱动测试通过。

## 2026-08-12（工作负载根 CA 多运行时兼容）

- 扩展 `w7.cc/inject-root-ca` Webhook 注入，为 curl、Python requests、Node.js、Git、AWS SDK/CLI 和 gRPC 补充各自的 CA 环境变量，PHP 容器内 curl 无需修改代码即可信任集群自建 CA。
- 保留 OpenSSL 系统证书目录及用户显式配置的 `SSL_CERT_DIR`，并继续覆盖注解工作负载已有的 CA 文件变量，避免使用旧证书路径。
- 影响模块：Kubernetes Pod Admission Webhook、根 CA 注入测试及后端使用说明。
- 验证：`go test ./common/service/k8s/webhook` 通过。

## 2026-08-25（依赖启动参数透传）

- ZPK 制品依赖增加 `startParams` 透传字段，使传统应用指定的环境版本可以随依赖安装配置返回。
- 影响模块：ZPK Manifest 解析与安装配置。
- 验证：依赖字段格式检查通过；完整 logic 包测试受工作区既有 `repo_domain_test.go` 参数签名过期阻断。
