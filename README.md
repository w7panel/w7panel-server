# W7Panel 后端

基于 Go + Gin 的 Kubernetes 云原生应用管理平台后端服务。

## 技术栈

- **Go 1.26** - 编程语言
- **Gin** - Web 框架
- **w7-rangine-go** - 应用框架
- **Kubernetes** - 容器编排

## 项目结构

```
w7panel/
├── app/                         # 业务应用模块
│   ├── application/             # 控制台、HTTP 控制器等基础应用能力
│   ├── auth/                    # 认证授权相关功能
│   ├── k3k/                     # K3K 虚拟集群管理
│   ├── k3s-registry/            # K3s 镜像仓库和容器运行时相关能力
│   ├── metrics/                 # 监控指标相关功能
│   └── zpk/                     # 应用包、站点和部署逻辑
├── common/                      # 公共基础模块
│   ├── accessor/                # 数据访问接口封装
│   ├── dao/                     # 数据访问对象
│   ├── entity/                  # 数据实体定义
│   ├── helper/                  # 通用工具函数
│   ├── middleware/              # HTTP 中间件
│   └── service/                 # Kubernetes、Helm、镜像仓库等公共服务
├── dev-tools/                   # 开发、构建、测试和代码生成工具
│   ├── .cnb.yml                 # CNB 构建流水线配置
│   ├── hack/                    # 代码生成等开发辅助脚本
│   │   └── codegen/             # Kubernetes client/codegen 工具链
│   └── scripts/                 # 本地构建、启动、清理、测试和 CLI 包装脚本
├── docker/                      # 镜像构建相关实验和 Kaniko 配置
├── k8s/                         # 项目自定义 Kubernetes API 和生成客户端
│   ├── crds/                    # CRD 源定义
│   └── pkg/                     # API 类型、clientset、informer、lister 等代码
├── kodata/                      # 运行时静态资源目录
│   ├── assets/                  # 前端静态资源和图标
│   ├── charts/                  # 内置 Helm Chart 包
│   ├── crds/                    # 内置 CRD 清单
│   ├── plugin/                  # 插件和编辑器资源
│   ├── shell/                   # 运行时安装、升级和集群操作脚本
│   ├── wasm/                    # Wasm 插件资源
│   └── yaml/                    # 内置 Kubernetes YAML 模板
├── config.yaml                  # 默认配置文件
├── go.mod                       # Go 模块依赖定义
└── main.go                      # 服务启动入口
```

## 开发约定

- 第三方依赖包统一通过 Go Modules 管理，新增依赖使用 `go get` 并提交 `go.mod`、`go.sum` 的变更。
- 禁止将第三方依赖源码复制到本项目目录下使用；确需修改第三方代码时，应优先通过上游 PR、fork 后使用 `replace` 或独立模块方式处理。
- 涉及操作系统、CPU 架构或容器运行时差异的代码，必须使用 Go build tag 隔离平台相关实现，避免影响其他平台编译。
- Linux-only 能力应放在 `//go:build linux` 文件中，并提供必要的非 Linux stub 或降级实现，保证本项目在非 Linux 开发环境下可编译。
- 开发、调试、构建、代码生成和一次性维护脚本统一放在 `dev-tools/` 下，不要散落在业务目录或项目根目录。
- `kodata/` 只存放运行时需要随程序分发的静态资源、Chart、CRD、脚本和模板，禁止放置本地构建产物、临时文件、缓存文件或开发工具输出。

## 快速开始

### 环境要求

- Go 1.26+
- Kubernetes 集群

### 开发模式

```bash
# 设置环境变量
export BASE_DIR=/home/wwwroot/w7panel-dev

# 编译
cd $BASE_DIR/w7panel
go build -o ../dist/w7panel .

# 启动服务
cd $BASE_DIR/dist
CAPTCHA_ENABLED=false \
LOCAL_MOCK=true \
KO_DATA_PATH=$BASE_DIR/dist/kodata \
KUBECONFIG=$BASE_DIR/kubeconfig.yaml \
./w7panel server:start
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LOCAL_MOCK` | true | 开发模式 |
| `CAPTCHA_ENABLED` | true | 验证码开关 |
| `KO_DATA_PATH` | ./kodata | 静态资源路径 |
| `KUBECONFIG` | ./kubeconfig.yaml | K8S 配置 |
| `W7PANEL_HTTP_SERVER_PORT` | 8000 | HTTP 端口 |
| `BOOTSTRAP_ALLOWED_SOURCE_HOSTS` | - | 额外允许的预装制品源主机，多个值以逗号分隔；内置允许 `zpk.w7.cc` 和 `zpk.fan.b2.sz.w7.com` |

## 主要功能

- **WebDAV 文件管理** - 容器内文件在线管理
- **压缩/解压** - 支持 zip, tar, tar.gz, tar.xz
- **权限管理** - chmod, chown 操作
- **应用部署** - Helm, Docker Compose, YAML
- **预装制品协调** - 通过 BootstrapProfile 同步 BootstrapInstallation，按依赖、版本与并发策略复用 ZPK/AppGroup 安装
- **制品订单通知** - 安装时将应用域名和规范化应用标识写入制品 ticket，并在安装完成后通过 ticket 传给市场订单链路
- **制品安装冲突处理** - 配置读取和安装接口统一解析仓库返回的订单绑定冲突；域名冲突返回原绑定域名，应用引用冲突返回原面板地址及原应用标识，支持跳转原面板定位应用或在用户确认后以受控 `reinstall` 覆盖旧绑定；升级始终校验应用标识
- **应用外部服务** - AppGroup 可声明多个中立服务入口，供续费、授权、工单等外部系统接入
- **集群管理** - 节点、资源对象管理
- **网关插件权限** - 为创始人默认权限注册网关插件查看、新建、编辑和删除菜单权限
- **插件微应用入口过滤** - 顶部微应用接口根据 MicroApp 的 `w7.cc/manifest-type=gateway-plugin` 注解排除插件，避免出现在顶部菜单和“应用直达”列表

### BootstrapProfile 预装制品

控制器在 `k8s.watch=true` 时随共享 Controller Manager 启动。BootstrapProfile 和 BootstrapInstallation 统一使用 `w7panel.w7.com/v1alpha1` API Group。BootstrapInstallation 只有在对应 AppGroup 同时满足 `status.ready=true` 和 `status.deployStatus=deployed` 时才进入 Ready；安装 Lease 会持有到真实部署完成、失败或超时。Lease 抢占、续租、释放和并发槽已统一复用 `common/service/k8s/coordination/`，详细设计见 [Kubernetes Lease 协调组件](../docs/src/development/k8s-coordination.md)。CRD 清单位于 `kodata/crds/w7panel.w7.com_bootstrapprofiles.yaml` 和 `kodata/crds/w7panel.w7.com_bootstrapinstallations.yaml`，字段、状态机和示例见 [BootstrapProfile 预装制品方案](../docs/src/development/bootstrap-installation.md)。

`spec.strategy.maxRetries` 未填写时默认重试 3 次；显式设置为 `0` 时不重试。

当前自动安装执行器仅支持 HTTPS ZPK 源。`type` 作为执行器扩展点，未填写时兼容默认为 `ZPK`，当前其他类型会在 Profile 校验阶段被拒绝。Profile 里声明的每个 installation 都会安装，不再使用 `enabled`/`required`。ZPK 可通过 `installOptions.helmValues` 提供内部 Helm 首次安装参数。已存在对应 AppGroup 时 Bootstrap 直接跳过，不执行自动升级。用户主动删除 AppGroup 后不会自动重装，直到 Profile revision 再次更新。OCI 地址仍未开放执行。内置允许 `zpk.w7.cc` 和 `zpk.fan.b2.sz.w7.com`，其他 HTTPS 主机需显式配置：

Bootstrap Controller 使用 ServiceAccount Token 作为 ZPK 安装的 Kubernetes 访问凭证，但不会将其作为面板用户身份写入 AppGroup 的 `w7.cc/create-username` 或 `w7.cc/create-role` Label。

```bash
export BOOTSTRAP_ALLOWED_SOURCE_HOSTS=zpk.example.com,registry.example.com
```

## 维护命令

ZPK 仓库信息接口返回 `data.external_services` 时，安装器会将有效入口写入根 AppGroup 的 `spec.externalServices`。

```bash
# 将旧 kube-system/coredns-custom 私有 DNS 配置迁移为 PrivateDNS CRD
w7panel privatedns-upgrade

# 如需用旧 CoreDNS 配置覆盖已有 PrivateDNS CRD
w7panel privatedns-upgrade --overwrite
```

### 内置应用与旧版 Higress 插件迁移

升级脚本会应用 `kodata/yaml/bootstrap-profile.yaml`，由 BootstrapProfile 安装以下内置应用：

- `w7panel-pluginwhitedomain`：域名插件
- `w7panel-pluginratelimit`：限流插件
- `w7panel-cloudnoauth`：CloudNoAuth

随后自动将历史内置 WasmPlugin 的用户配置迁移到制品资源。维护命令为：

```bash
# 创建或更新内置预装清单
kubectl apply -f "$KO_DATA_PATH/yaml/bootstrap-profile.yaml" --server-side

# 查看安装状态
kubectl get bootstrapprofile w7panel-default
kubectl get bootstrapinstallation -l w7.cc/bootstrap-profile=w7panel-default

# 等待制品资源就绪并迁移旧配置
DOMAIN_TARGET_GROUP=w7panel-pluginwhitedomain \
RATE_LIMIT_TARGET_GROUP=w7panel-pluginratelimit \
DELETE_LEGACY=true \
sh "$KO_DATA_PATH/shell/upgrade-wasm-plugins.sh" all
```

基础 Higress 由集群安装流程预先提供，不属于该 Profile。修改内置应用清单时必须同步递增 BootstrapProfile 的 `spec.revision`。迁移脚本会备份旧资源、迁移全局及域名规则配置、切换插件并校验结果，失败时自动恢复旧插件。面板自动升级在校验成功后会删除旧资源；新集群没有旧资源时，只为制品插件补充稳定的逻辑标签。手工执行脚本未设置 `DELETE_LEGACY=true` 时，仍会保留已停用的旧资源，便于调试。

集群升级会对已达到最大重试次数并进入 `Failed` 的内置 BootstrapInstallation 发起非阻塞重建，使相同 Profile revision 在下一次集群升级时也能重新尝试安装。Profile revision 发生变化时，Controller 会先清理上一轮由 Bootstrap 拥有的失败 AppGroup，再开始新一轮安装。

## API 接口

详见 [API 文档](../docs/api/README.md)

## 测试

```bash
# 运行 API 测试
cd $BASE_DIR/tests
bash webdav.sh

# 运行压缩功能测试
bash compress.sh
```

## 相关文档

- [部署文档](../docs/deployment/README.md)
- [开发指南](../docs/development/README.md)
- [测试文档](../docs/testing/README.md)
