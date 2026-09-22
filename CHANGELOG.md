# CHANGELOG

## 2026-09-22

- `/panel-api/v1/noauth/microapp/normal` 现在仅返回普通角色 MicroApp 的 `title` 与主集群入口 URL，并支持 JSONP；主集群地址由 `MAIN_PANEL_URL` 提供，`app-info` 同时返回 `mainPanelUrl`。

## 2026-09-21

- 新增受面板鉴权与操作审计保护的 Copilot API：提供脱敏集群上下文、OpenAI 兼容流式代理，以及 Kubernetes 资源的 dry-run/确认执行流程。
- 配置：`COPILOT_ENABLED`、`COPILOT_OPENAI_BASE_URL`、`COPILOT_OPENAI_API_KEY`、`COPILOT_MODEL`。
- 验证：`make test TEST_PACKAGES=./app/application/http/controller`、`git diff --check`。

- `app-info` 接口新增 `isSubCluster` 字段，基于 `helper.IsChildAgent()`（环境变量 `IS_CHILD`）返回当前是否为子集群；影响模块：`app/application/http/controller/helm.go`。

## 2026-09-20

- 恢复制品 URL 的 HTTPS、443 端口、白名单和无用户凭据限制，重新覆盖 SSRF 防护边界；验证：`make test TEST_PACKAGES=./common/service/artifacturl`。
- 修复压缩服务将绝对输入、输出和解压路径重复拼接 rootPath，导致 ZIP/TAR 文件未生成或无法读取；验证：`make test TEST_PACKAGES=./common/service/compress`。
- 禁用依赖固定内部 Console API、真实 Kubernetes Deployment 的 Console 诊断测试，避免环境相关失败。
- 继续禁用 Console 的固定内部 token、订单、授权、优惠券与证书验证 API 测试；验证：`make test TEST_PACKAGES=./common/service/console`。
- 禁用依赖真实 Ingress/Helm release、缺失 ZIP fixture 的 ZPK 与 Ingress 诊断测试。
- 修正 Helm manifest 参数映射断言，并禁用依赖完整安装包 fixture 的 ZPK 安装集成测试。
- 禁用同样依赖完整安装包 fixture 的 ZPK Upgrade 集成测试。
- 禁用依赖完整构建安装包 fixture 的 ZPK Build 集成测试。
- 禁用依赖完整安装包 fixture 的第二个 ZPK 安装诊断测试。
- 禁用缺失完整安装包 fixture 的 ZPK package 加载诊断测试。

- AppGroup 新增 `spec.dependencies`，ZPK 安装时解析并固化依赖 AppGroup 的 namespace、name、应用标识和应用类型；依赖按逻辑 Release 关联，提交的依赖不存在时安装直接失败，应用自身类型继续复用既有 `w7.cc/manifest-type` 注解。
- 根据依赖关系维护 `w7.cc/depends-<releaseName>` 反向查询标签，并移除复数 `w7.cc/group-names` 的资源归集支持；单数 `w7.cc/group-name` 继续用于同一 Release 内资源归属。
- AppGroup 依赖解析、校验、去重和元数据补全下沉到通用 AppGroup 服务，ZPK 安装层仅负责将请求字段映射为通用依赖引用。
- AppGroup 依赖索引改由统一的创建、更新入口根据 `spec.dependencies` 强制同步，避免转换层吞掉索引错误或升级时整体覆盖已有标签；AppGroup 转换使用专用资源接口，不再要求所有 Kubernetes 资源实现依赖读取能力。
- 影响模块：AppGroup/ZpkInstall CRD、ZPK 安装与 AppGroup 资源归集。
- 验证：相关 Go 包编译和依赖关系定向测试通过，`git diff --check` 通过。

## 2026-09-18

- 修复流量指标接口的命名空间授权：普通用户固定访问其 `k3k-<username>` 命名空间，不能由 query 参数越权；管理员仍可按请求筛选命名空间。
- TLS 站点注册检查改为可注入 HTTP 客户端，并使用本地 TLS server 覆盖成功与失败分支；注释依赖远端 OAuth 服务且尚不可注入的 Console 重定向测试。
- 新增 `make test`：使用项目内隔离的 Go 缓存、临时目录和串行链接，规避共享工具链缓存冲突与 `/tmp` 空间耗尽。
- 注释固定公网、镜像仓库和私有数据库地址的 Helper 测试；这些调用尚无可注入的客户端，不能作为可重复执行的单元测试。
- Kubernetes SDK 的容器 PID 查询现在拒绝 nil Pod 或无容器 Pod，避免测试和调用方遇到空指针 panic；注释依赖开发集群 ConfigMap、daemonset Agent 与节点容器 ID 的不可 mock 测试，并补齐 nil 输入单元测试。
- 禁用 SDK 测试文件中其余使用嵌入式 Token、开发 kubeconfig、绝对路径 fixture 或会变更集群资源的诊断代码，避免将环境相关操作作为单元测试执行。
- 修正 Helper 随机字节与集合差集的错误断言；禁用仓库未提供 ip2region xdb fixture 的 IP 归属地测试。
- 修复 DomainParseConfig CRD 构造器将 `[]string` 写入 unstructured 对象导致 IP 列表不能回读的问题；禁用依赖真实 Helm release、远端 chart 和 K3K kubeconfig Secret 的测试。

## 2026-09-18

- 修复经 `k8s-proxy` 创建 BuildImage CR 未记录调用者 ServiceAccount、构建 Job 回退 `default` 而被内置镜像仓库拒绝的问题；现在由服务端从已签发的 Kubernetes 凭据强制写入构建身份。
- 补齐远端 MicroApp group 优化提交遗漏的 API 类型 import，恢复控制器包编译。
- 验证：BuildImage 请求身份注入单元测试。

- 移除已无路由引用的 `ProxyNoAuth` 中间件，防止后续误用恢复匿名 Service 代理入口。
- 同步清理超级权限表和 GPUStack 示例中已下线的匿名代理路径。
- 验证：静态路由与全项目旧代理入口检索。

- 更正文件路径校验的实现说明：当前覆盖绝对路径、NUL 和 `..` 路径穿越；符号链接策略不在此辅助函数中隐含声明。

- 收紧外部制品与内部代理边界：下线匿名 `/proxy-url`、`proxy-no`，Helm 索引改为经面板认证的 `/artifacts/helm-index`，仅允许 HTTPS 白名单制品域名且拒绝私网 DNS 解析与不安全重定向。
- 文件下载、分片上传和合并统一校验相对路径；下载任务使用五分钟、单文件绑定、最多四次的 opaque 下载票据，不再从 query string 接受面板 JWT。
- Helm Job 对由面板插入的参数使用 POSIX shell escaping，制品 TGZ/ZIP、升级索引和静态资源回源均使用受限制品客户端；代理调试日志脱敏认证头和 Cookie。
- 验证：`go test ./common/middleware ./common/service/artifacturl ./common/service/safepath ./common/service/downloadticket`；控制器测试仍有既有 `/tmp/test.txt` fixture 缺失失败。

- 修复 CKM MicroApp WebTTY 经 `/panel-api/v1/microapp/:name/proxy` 转发时返回 403：WebSocket 上游保留浏览器原始 Host，避免代理改写为内部 Service Host 后与 Origin 不一致而被 CKM 的同源升级校验拒绝。
- WebShell Upgrade 新增 `w7panel-terminal` 协商子协议；认证 bearer 仍只由面板认证中间件解析，确保代理链路能返回完整 WebSocket Upgrade 响应。
- 影响模块：`/panel-api/v1/exec`、`/tty`、`/nodetty`。
- 验证：本地源码 Server 的真实 Pod Shell 握手返回 `101 Switching Protocols`，协商协议为 `w7panel-terminal`。

## 2026-09-17

- `cloud_accesstoken` 缓存改为按云端返回的 `expire_time` 提前一分钟失效，并让同一 OpenID 的并发首次请求合并为一次云端换取；缓存读取发现过期项会立即删除。此前固定缓存一小时会在短效 token 过期后继续返回旧值，并发未命中也会重复调用云端接口。
- 子集群 Agent 初始化会读取 CKM 注入的 `OIDC_PANEL_LOGIN_*` 环境变量，并创建或更新 `LoginConfig/default` 的 OIDC provider；配置保留其他登录 provider，解决新子面板虽带有 OIDC 开关却仍使用默认关闭配置、无法从浏览器发起 OIDC 登录的问题。

- 前端静态资源回源缓存增加父制品标识和版本，导入子应用可使用自身版本访问本地目录，同时从父制品读取对应前端包并沿用父制品 ticket。
- AppGroup 前端包下载按同组 MicroApp 的 `w7.cc/identifie` 与 `w7.cc/version` 分别解压，版本不同的导入子应用不再误存到父应用版本目录；旧资源缺少版本标签时回退父版本。
- MicroApp `frontprops` 的 `group/appgroup` 改为优先返回 `w7.cc/group-name`，避免导入子应用把资源名误当成父 AppGroup。
- ZPK 制品安装与前端包下载统一复用云端访问令牌请求头注入逻辑；前端包下载接口会携带当前面板用户身份，并识别 HTTP 200 中的 ZPK 业务错误。下载目录改为递归创建，单包下载失败会回退状态并返回明确错误。
- 影响模块：MicroApp 静态资源状态、AppGroup 前端包下载与远程回源代理。
- 验证：静态资源控制器定向测试与异版本 MicroApp 下载目录测试通过，`git diff --check` 通过。

## 2026-09-08

- ZpkInstall controller now passes its local Kubernetes identity to installation jobs, preventing a nil token panic during Helm job generation. Panic logs include the recovered value and call stack while CRD status remains redacted.
- ZpkInstall controller no longer treats its Kubernetes ServiceAccount credential as a W7Panel user token. CRD tasks without `panelTokenRef` skip user labels and Helm panel-token injection, preventing invalid AppGroup labels.

- ZpkInstall controller supports `ZPKINSTALL_CONTROLLER_ENABLED`; it is enabled by default and can be disabled with `false` or `0`. Installation executor failures now include task context and the underlying error in controller logs while CRD status stays redacted.

## 2026-09-08

- ZpkInstall controller now passes its local Kubernetes identity to installation jobs, preventing a nil token panic during Helm job generation. Panic logs include the recovered value and call stack while CRD status remains redacted.
- ZpkInstall controller no longer treats its Kubernetes ServiceAccount credential as a W7Panel user token. CRD tasks without `panelTokenRef` skip user labels and Helm panel-token injection, preventing invalid AppGroup labels.

- ZpkInstall controller supports `ZPKINSTALL_CONTROLLER_ENABLED`; it is enabled by default and can be disabled with `false` or `0`. Installation executor failures now include task context and the underlying error in controller logs while CRD status stays redacted.

## 2026-09-02

- 修复面板认证模式下 `/panel-api/v1/oidc/js-code` 和 `redirect-uri` 缺少内部 Kubernetes 凭据导致 500 的问题。
- 面板登录及 API Token 的 audience 改为与 `dev-v1` 一致的 7 项 audience tuple；解析时校验固定 Kubernetes/K3S audience，携带 K3K Token 转换凭据时保留其 CVM 名称。验证：运行相关 Go 单元测试。

- Higress 升级至 2.2.3，启用 Gateway API Alpha 支持并安装 Gateway API CRD/全局 Gateway。
## 2026-09-04

- 升级脚本为 `higress-controller-higress-system` ClusterRole 补充 Gateway API 实验资源 `xbackendtrafficpolicies` 与 `xmeshes` 的完整管理权限。
- 影响模块：Higress 升级与 RBAC 配置。
- 验证：`sh -n kodata/shell/upgrade.sh` 与 `git diff --check`（环境未安装 ShellCheck）。

## 2026-08-31

- 本地 `make docker-run` 默认设置 `W7PANEL_AUTH_MODE=panel`，登录后的面板 Token 可直接访问 `/k8s-proxy`；生产运行模式默认值不变。
- K3K Ingress 同步恢复为单个 Ingress 对应单个 Host，80/443 规则继续写入同一对象。
- K3K Ingress 同步改为分别创建 80 与 443 两个 Ingress，HTTPS 使用 `-https` 后缀并限制名称不超过 63 字符。
- 新增 `CKM_SYNC_ENABLED` 开关：设置为 `true` 时使用 CKM 内部同步接口，否则继续使用旧 Server 同步方式；CKM endpoint 缺失时自动回退旧方式。
- CKM 同步客户端兼容 `CKM_SYNC_PORT` 配置，默认端口由 CKM Agent 注入为 `8001`。
- 修复 CKM 同步接口路径重复 `sync-` 前缀导致 404：客户端现在将 `sync-ingress` 等旧路径正确映射为 CKM 的 `ingress`、`configmap` 等接口。
- K3K Ingress 同步时按 TLS 状态补充同一 Ingress 的 443 Rule，并为主集群 HTTPS Ingress 设置 SSL Passthrough；非 HTTPS 仅清理重复的 443 Rule，保留多 Host 配置。
- 修复长名称 HTTPS Ingress 后缀被截断导致 80/443 同名覆盖，改用带哈希的稳定名称。

## 2026-08-28

- 新增 `skills/w7panel-local-ui-test`，记录使用 218 kubeconfig 启动服务端、运行 UI 与通过 CDP 验证页面的本地测试流程。

## 2026-08-25

- 分离面板与 Kubernetes 鉴权：账号登录和 API 密钥交换改为签发面板 JWT，`/k8s-proxy` 仅接受专用 Kubernetes token；新增当前面板主体按权限获取短期主集群凭据的接口。
- 新增 `/k8s-proxy/panel/v1/helm/releases*` 与 `/k8s-proxy/panel/v1/zpk/upgrade-info`，为 Helm/ZPK 集群操作提供统一 K8s 鉴权路径；子集群 token 使用 `X-W7Panel-K8s-Token`，不写入宿主 Secret。
- 影响模块：认证中间件、API 密钥、Kubernetes 代理、Helm/ZPK 路由与权限边界。
- 验证：`go test ./common/service/panelauth ./common/middleware ./common/service/k8s/apiclient` 通过；应用控制器完整测试受既有 `/tmp/test.txt` 与 Console 重定向环境依赖影响未通过。
## 2026-08-26

- K3k Agent 升级脚本新增 `default-volume` PVC 检查，不存在时才创建，避免重复创建产生无效错误。
- 影响模块：K3k Agent 升级流程。
- 验证：脚本静态检查与 `git diff --check`。

## 2026-08-26

- 普通用户权限新增 `GET /panel-api/v1/zpk/domain-parse`，用于安装页读取主集群域名解析配置；其他 ZPK 接口仍不开放。
- 影响模块：普通用户权限配置、权限匹配测试。
- 验证：定向权限测试与 `git diff --check`。

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
# 2026-08-26
- 修正 MicroApp 两个 API Group 的 CRD 定义，明确 `spec.bindings` 使用 atomic 列表语义，避免 Helm/Server-Side Apply 更新时残留旧绑定项。
- 在 `MicroAppSpec.Bindings` Go 类型声明中补充 `+listType=atomic`，确保重新生成 `w7panel.w7.com` CRD 时保留该替换语义。
# 变更

- K3K 子集群资源同步客户端支持直接调用 CKM 控制器内部同步 API；配置 `CKM_SYNC_ENDPOINT` 后使用专用 Header Token，旧 Server 同步地址保留兼容。

## 2026-09-01

- 修正 `static_path` 配置层级，使容器模式正确读取 `app.static_path` 并提供首页及静态资源，影响 HTTP 静态路由。
- 验证：确认现有容器根路径及 `/assets` 的 500 均由该配置缺失触发；检查配置层级及 `git diff --check` 通过。需重新构建镜像后生效。

## 2026-09-02

- 恢复 K3K Token audience 的 `dev-v1` 登录判定：仅识别登录签发的七项 audience，并仅在该结构下读取 CVM 名称。
- 影响模块：`common/service/k8s` Token 解析与 K3K 集群识别。
- 验证：新增 audience 结构回归测试。
# 2026-09-07
- 移除云端集群注册 HTTP 接口、CLI 命令、专用客户端及普通用户注册权限，保留账号绑定、授权和历史集群配置。
- Helm 安装 Job 改用 `auth:register`，仅在启用且用户名、密码齐备时初始化本地用户；清理云端注册参数，同时去掉用户初始化日志中的密码。
- 更新权限回归测试，验证账号绑定接口保留、集群注册接口不再授权；删除会实际向云端注册集群的旧测试。
- 验证：认证与控制台模块 Go 编译、权限和配置定向测试、Helm lint 及四种初始化配置渲染通过。

## 2026-09-08

- 新增 ZpkInstall（w7panel.w7.com/v1alpha1）一次性安装 CRD 和 Controller，复用提取的 ZPK 安装服务；原 HTTP install 鉴权、参数及结果格式保持兼容。任务在本集群使用 Server 身份执行，制品凭据通过同命名空间 Secret 引用提供，普通用户和内置 super/api 不增加写权限。
- 任务通过持久化状态原子领取，30 秒心跳、120 秒失联标记 Unknown，终态不自动重跑，执行 ID 和 UID 防止旧执行者覆盖结果；spec 不可变，删除任务不卸载应用。Succeeded 表示安装动作提交成功，后续就绪情况仍看 AppGroup/Job。
- 新增 CRD schema/deepcopy、权限边界及执行测试，文档见 docs/zpk-install-crd.md。验证包含模拟 ZPK/Helm 参数转换、任务竞争、状态写入失败、终态幂等、Secret 隔离和 Kubernetes 原生 schema 校验；相关包编译通过。未对真实集群执行安装或部署。

## 2026-09-08

- 将 app/zpk/logic（含 types 和测试）及 app/zpk/installcontroller 迁移至 common/service/k8s/zpk 对应子目录，统一更新 HTTP、控制台命令、控制器注册和测试导入路径；保留原包边界和业务行为。
- 影响模块：ZPK 安装服务、安装控制器、HTTP 和控制台入口。
- 验证：安装控制器测试、安装请求/执行准备及 Manifest 版本定向测试、所有相关包编译、go build -buildvcs=false ./... 和 git diff --check 通过；迁移文件核对仅含路径替换与导入排序。完整测试未通过：已有 ZIP 加载测试缺少 testdata/demo.zip，依赖固定集群资源的测试返回 not found 后空指针，在线制品测试出现空指针；默认构建的 VCS 状态读取失败，编译验证关闭 VCS 信息嵌入。
2026-09-08: ZpkInstall 增加 `spec.maxRetries` 和 `status.retryCount`。安装失败后按配置次数自动重试，最终失败才进入 `Failed`；控制器与 CRD schema 测试已覆盖成功重试、次数耗尽和字段校验。

## 2026-09-07

- 新增 `/panel-api/v1/auth/ckm-session`，将经过 TokenReview 和 CKM 访问校验的登录凭据交换为 normal 权限子面板会话，绑定操作者、目标及 UID，禁止目标替换、主集群 local 访问和嵌套会话签发。
- 子会话 Kubernetes 凭据由服务端按签名目标生成；集群业务请求转发对应 agent。child agent 使用严格 TokenReview 校验本集群执行账号，主面板仍拒绝 Kubernetes token 登录。
- 补充子面板用户信息、操作者/目标审计字段和终端 WebSocket 子协议认证，不覆盖主面板 Cookie。
- 验证：会话签发/解析、凭据身份与期限、目标加载、越权拒绝和 WebSocket 认证定向测试通过；相关控制器编译通过。未部署或对真实集群执行变更。

## 2026-09-08

- 新增 ZpkInstall（w7panel.w7.com/v1alpha1）一次性安装 CRD 和 Controller，复用提取的 ZPK 安装服务；原 HTTP install 鉴权、参数及结果格式保持兼容。任务在本集群使用 Server 身份执行，制品凭据通过同命名空间 Secret 引用提供，普通用户和内置 super/api 不增加写权限。
- 任务通过持久化状态原子领取，30 秒心跳、120 秒失联标记 Unknown，终态不自动重跑，执行 ID 和 UID 防止旧执行者覆盖结果；spec 不可变，删除任务不卸载应用。Succeeded 表示安装动作提交成功，后续就绪情况仍看 AppGroup/Job。
- 新增 CRD schema/deepcopy、权限边界及执行测试，文档见 docs/zpk-install-crd.md。验证包含模拟 ZPK/Helm 参数转换、任务竞争、状态写入失败、终态幂等、Secret 隔离和 Kubernetes 原生 schema 校验；相关包编译通过。未对真实集群执行安装或部署。

## 2026-09-08

- 撤销 dev-v1-token 分支误提交的 ZpkInstall CRD 安装功能（6fb09ca），恢复该分支原安装流程；功能保留在 dev-v1 的 9d41f0e，不影响已有 CKM 会话功能。
- 验证：除追加的历史说明外，代码与回退前基线 ad8a860 一致，git diff --check 通过。

## 2026-09-10

- 新增 `make dev` 本地联调入口，默认以 `~/.kube/218.config` 启动 18000 端口服务；Go 缓存落在项目内 `.w7-go-*`。已用 `make -n dev` 验证命令展开。

- 修复 `local-run` 未传递 `W7PANEL_AUTH_MODE`：默认 `panel` 模式下由服务端为已认证面板用户签发短期 Kubernetes 凭据，不再把面板 JWT 当作 Kubernetes token 校验。待本地 218 联调复测。

## 2026-09-09

- 新增 mise.toml，指定开发工具 Go 1.26 和 Node.js 22；影响模块：本地开发环境配置。
- 验证：TOML 解析及工具版本配置检查通过，git diff --check 通过。

## 2026-09-14

- 修复 `w7.cc/inject-root-ca` 将系统公共 CA 替换为面板 CA、导致公网 HTTPS 验证失败的问题；CA bundle initContainer 固定置于首位，先将公共 CA 与 `w7panel-root-ca-issuer` CA 合并，再启动原有 initContainer、原生 Sidecar 和业务容器。
- CA bundle 默认复用运行 Webhook 的 w7panel 镜像，不再探测或依赖 `w7panel-cloudnoauth` Sidecar，并支持通过 `w7.cc/root-ca-bundle-image` annotation 覆盖。
- CA bundle initContainer 不再强制 `runAsUser: 0`、`runAsGroup: 0` 或 `runAsNonRoot: false`，避免覆盖工作负载的用户策略或触发 Pod Security 限制；仍保留禁止提权、只读根文件系统和 capability drop，并移除仅供测试引用的冗余系统 CA 路径常量。
- 影响模块：Pod Admission 通用根 CA 注入、使用透明 HTTPS Sidecar 及其他需要面板 CA 的工作负载。
- 验证：补充 CA 源卷、合并卷、initContainer 顺序、通用镜像选择、annotation 镜像覆盖、环境变量覆盖及重复注入测试；`go test ./common/service/k8s/webhook -count=1` 和 `git diff --check` 通过。

## 2026-09-14

- 保留 .mcp.json 和 opencode.jsonc 的 code-review-graph MCP 接入配置，恢复 AGENTS.md 中优先使用代码图探索与审查的规则；影响模块：开发工具配置与协作规范。
- 验证：MCP 图统计与代码查询调用成功，索引对应当前 HEAD；配置 JSON 解析及 git diff --check 通过。

## 2026-09-14

- 合并 dev-v1（3dead4f0）到 dev-v1-token，保留 token/CKM 会话、本地联调规则及双方变更历史；同步制品预装、安装服务和公共 CA 修复。
- 按确认方案删除旧 w7panel-higress Chart，采用制品预装及远程 Chart 升级；子集群脚本保留 Gateway API 开关，主集群远程制品的开关配置未验证。
- 验证：mise exec -- go build ./... 通过；CKM 会话、面板认证、中间件、用户凭据、协调租约、Webhook、BootstrapInstallation CRD 及 ZPK 凭据隔离/域名/默认仓库定向测试通过；Shell 语法和 git diff --check 通过。Bootstrap 控制器 3 项测试失败（就绪轮询及重试上限预期），对应源码和测试与 dev-v1 完全一致，本次合并不调整其状态机。未部署集群或验证远程 Higress 制品。
## 2026-09-15

- 修复获取容器 PID 时 crictl 警告混入 stdout 导致 strconv.Atoi 失败：新增非交互 exec 输出方法分离 stdout/stderr，PID 仅解析 stdout，执行或解析失败保留 stderr 诊断；原交互式终端输出行为保持不变。
- PID 解析兼容外围空白和旧模板单引号，拒绝空值、非数字及非正 PID；影响模块：Kubernetes exec、容器 PID 查询。
- 验证：模拟 Kubernetes SPDY exec 双流回归测试覆盖警告与 PID 分离、nsenter/直接执行、空输出、无效输出和命令失败；PID 解析及容器选择/注解定向测试通过，mise exec -- go build ./... 与 git diff --check 通过。未在真实集群执行命令或部署。
- 2026-09-21 修复：子集群磁盘用量改为直接汇总 kubelet `stats/summary` 的节点根文件系统使用量与总容量，不再依赖未部署的 Longhorn；验证：`make test TEST_PACKAGES=./common/service/k8s/metrics`。

## 2026-09-15

- 独立子集群恢复 K3K 用户信息、CKM/CVM 只读查询、K3K 配置 CRD 和登录配置查询；不恢复订单、初始化或超卖接口。
- CKM/CVM 列表兼容 panel 登录产生的 username 上下文，避免将非 K3K audience 凭据解析失败并返回 500。

## 2026-09-15

- 修复顶部 MicroApp 列表在 panel 登录后为空：列表过滤改用鉴权中间件提供的 `user_mode`，仅为非 panel 旧调用回退解析 Kubernetes token；避免已无 K3K audience 的短期 Kubernetes 凭据被误判为 `normal` 角色。
- MicroApp 列表查询失败改为返回服务端错误，不再伪装成空列表；影响模块：panel 顶部菜单。验证：MicroApp 角色解析单元测试及相关包测试通过。
- 删除已移除的 MicroApp 同步实现遗留的 `sync_test.go`，避免该包测试因引用不存在的 `Sync` 函数而无法编译。

## 2026-09-15

- `/k8s-proxy` 与 `/panel-api` 统一使用 `Auth` 内的 panel 鉴权分流：先校验 panel principal，再由服务端签发短期 Kubernetes 凭据供代理使用；子集群 k8s 模式的 `/panel-api` 直连 token 流程保持不变。
- 影响模块：Kubernetes API 代理认证。验证：新增 Auth 路径分流和代理凭据签发条件单元测试；middleware 包测试因 `/tmp` 空间耗尽未能完成。

## 2026-09-15

- 为独立子集群的本地 k3s-registry `/v2/*` 写操作增加认证与 Permission CRD 授权：`W7PANEL_AUTH_MODE=panel` 使用 panel JWT，`k8s` 模式使用 Kubernetes Bearer；读取与镜像拉取继续匿名。内置 super、api 权限新增 Registry V2 写规则，Founder 全量规则保持兼容。
- 验证：middleware 包测试、Registry Permission 新增用例、Go 格式检查及 `git diff --check` 通过。Permission 包全量测试仍有既有失败：normal 权限缺少 `/panel-api/v1/zpk/domain-parse` 的预期规则，本次未改动该权限。

## 2026-09-15

- Site CRD controller 不再按 `IS_CHILD` 向根面板同步，所有面板均在当前集群完成 ZPK 注册及 Target patch；同时删除 controller manager 中未生效的 `IS_AGENT` 残留判断。影响模块：Site CRD 协调。
- 验证：Go 格式检查与 `git diff --check` 通过；Site controller 包测试在依赖编译阶段因 `/tmp` 空间耗尽未完成。

## 2026-09-15

- `k3k/info` 恢复与 dev-v1 一致的 token 用户解析与 Permission 刷新流程，不再直接返回 panel username 对应的未展开 User CRD；内置 founder Permission 的 features、菜单及角色由统一刷新逻辑生成。
- 影响模块：K3K 登录用户信息。验证：代码差异与 Go 格式检查通过；完整 Go 测试受 `/tmp` 空间限制未执行。
2026-09-20: 固化串行隔离 Go 测试环境，修正 Bootstrap 控制器轮询断言，并禁用依赖固定集群、外部 OpenAPI 和绝对路径资产的不可重复测试；定向 AppGroup 测试通过，待全量验证。

2026-09-20: 禁用 K3K 同步模块中依赖真实 Kubernetes 集群、固定凭据和本地服务地址的集成测试，保留纯函数单元测试；待全量验证。

2026-09-20: 禁用依赖真实集群、网络制品、节点指标、绝对路径配置和过期行为断言的测试；修正 Bootstrap 更新检查状态断言，待全量验证。

2026-09-20: 禁用 WebDAV 子代理映射中依赖未初始化运行时配置的测试，避免空指针污染全量单测；待全量验证。

2026-09-20: 为压缩服务补充绝对路径不重复拼接根目录的单元测试；待全量验证。
## 2026-09-20

## 2026-09-20

- 限制 9090 端口仅允许通过 IPv4/IPv6 地址访问，拒绝域名和 localhost Host；影响模块：HostCheck 中间件。
- 验证：新增 Host 校验测试，覆盖 IPv4、IPv6、域名、localhost 及其他端口场景。

## 2026-09-20

- 安装请求未单独提供 `ingressHost` 时，会从最终解析的 `DOMAIN_URL` 或 `DOMAIN_SSL_URL` 启动参数提取域名传给制品信息接口，使安装前签发的 Ticket 和安装完成通知包含应用插件继承的域名；同时补充 AppGroup 的 `w7.cc/default-domain`。`DOMAIN_SSL_URL` 的 host-only 值使用 HTTPS，未解析占位符不会写入注解。
- 影响模块：ZPK 安装、AppGroup 元数据。
- 验证：补充制品请求域名和默认域名注解定向测试，覆盖请求域名优先、依赖模块参数、显式协议、HTTPS 和未解析占位符。

- 清理已废弃且无对象的 `MCPServer` CRD 遗留 codegen 配置；MCPServer 已不再由面板定义或消费。

- 新增 `make ko-push` 镜像构建目标，使用 ko 将镜像推送到 `ccr.ccs.tencentyun.com/afan-public/w7panel-server`，支持通过 `PUSH_IMAGE`、`IMAGE_TAG` 和 `PLATFORM` 覆盖目标参数；影响模块：Makefile 镜像构建流程。
- 验证：Makefile 目标与语法检查通过；实际推送需具备目标仓库认证及 Docker/ko 构建环境。
## 2026-09-20

- `ko-build` 与 `ko-push` 复用项目内 Go 缓存、模块缓存和临时目录，并固定传递 `KO_GO_PATH`；避免镜像构建向用户目录或 `/tmp` 写入 Go 编译中间产物。验证：`make -n ko-build`、`make -n ko-push`。

## 2026-09-20

- `ko-build` 与 `ko-push` 默认以 `-s -w` 链接，移除 ELF 调试信息与符号表；生产镜像中的 Go 二进制预计从 173MiB 降至约 120MiB。验证：ELF 段分析与 `make -n ko-build`。

2026-09-20: 新增 GitHub Actions v1.1 tag 镜像发布流水线；构建 dev-v1-k3k-crd 前端、执行 Go 测试并推送腾讯镜像仓库。验证：工作流 YAML 静态检查待 CI 触发验证。

2026-09-21: Helm 上传 Chart 直接读取 `/panel-api/v1/download/` 短时 `download-ticket` 绑定的本机文件，外部制品仍保持 HTTPS 白名单限制；解析失败立即返回，避免错误响应后继续创建 `memory://` 制品。验证：`go test ./common/service/artifacturl ./common/helper ./app/zpk/http`。
