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
- **预装制品协调** - 通过自包含的 BootstrapInstallation 按版本、重试和并发策略复用 ZPK/AppGroup 安装
- **制品订单通知** - 安装时将应用域名和规范化应用标识写入制品 ticket，并在安装完成后通过 ticket 传给市场订单链路
- **制品安装冲突处理** - 配置读取和安装接口统一解析仓库返回的订单绑定冲突；域名冲突返回原绑定域名，应用引用冲突返回原面板地址及原应用标识，支持跳转原面板定位应用或在用户确认后以受控 `reinstall` 覆盖旧绑定；升级始终校验应用标识
- **制品跨应用更新** - 新制品标识与原应用不同时，配置接口仍返回已有 AppGroup 名称，供安装界面读取原实例保存的参数
- **应用资源跟踪** - AppGroup Controller 自动为已归组的 workload 补齐 `w7.cc/group-name`，由 informer 持续同步 Deployment、StatefulSet、DaemonSet 等资源状态
- **集群管理** - 节点、资源对象管理
- **网关插件权限** - 为创始人默认权限注册网关插件查看、新建、编辑和删除菜单权限
- **插件微应用入口过滤** - 顶部微应用接口根据 MicroApp 的 `w7.cc/manifest-type=gateway-plugin` 注解排除插件，避免出现在顶部菜单和“应用直达”列表
- **顶部微应用角色判定** - 顶部入口只统计 `founder`、`super`、`normal` 面板角色对应的 Binding，功能菜单分组不参与多角色判定

### BootstrapInstallation 预装制品

控制器在 `k8s.watch=true` 时随共享 Controller Manager 启动。每个 BootstrapInstallation 直接声明制品、目标和执行策略，不再依赖 BootstrapProfile 或 revision。只有对应 AppGroup 同时满足 `status.ready=true` 和 `status.deployStatus=deployed` 时任务才进入 Ready；安装 Lease 会持有到真实部署完成、失败或超时。CRD 清单位于 `kodata/crds/w7panel.w7.com_bootstrapinstallations.yaml`，详细设计见 [BootstrapInstallation 预装制品方案](../docs/src/development/bootstrap-installation.md)。

`spec.strategy.maxRetries` 未填写时默认重试 3 次；显式设置为 `0` 时不重试。由 Bootstrap 创建的 AppGroup 部署失败或超时时，Controller 会先请求删除失败实例，待 AppGroup 标准卸载流程完成后重新安装；非 Bootstrap 所有的同名 AppGroup 不会被自动删除。

当前自动安装执行器仅支持 HTTPS ZPK 源。`type` 未填写时默认为 `ZPK`，其他类型会被 Installation 校验拒绝。ZPK 可通过 `installOptions.helmValues` 提供首次安装参数。已存在 identifie 一致的 AppGroup 时不执行自动升级。删除 BootstrapInstallation 只会卸载 `w7.cc/bootstrap-owner` 精确匹配当前 Installation UID 的 AppGroup，再由 AppGroup Controller 完成 Helm 卸载；不兼容旧 BootstrapProfile 所有权格式。内置允许 `zpk.w7.cc` 和 `zpk.fan.b2.sz.w7.com`，其他 HTTPS 主机需显式配置：

Bootstrap Controller 使用 ServiceAccount Token 作为 ZPK 安装的集群内 Kubernetes 访问凭证，但不会通过 `X-W7Panel-Token` 将该凭证发送给 ZPK 制品源，也不会将其作为面板用户身份写入 AppGroup 的 `w7.cc/create-username` 或 `w7.cc/create-role` Label。

```bash
export BOOTSTRAP_ALLOWED_SOURCE_HOSTS=zpk.example.com,registry.example.com
```

## 维护命令

```bash
# 将旧 kube-system/coredns-custom 私有 DNS 配置迁移为 PrivateDNS CRD
w7panel privatedns-upgrade

# 如需用旧 CoreDNS 配置覆盖已有 PrivateDNS CRD
w7panel privatedns-upgrade --overwrite
```

### 内置 BootstrapInstallation

升级脚本会应用 `kodata/yaml/bootstrap-installations.yaml`，安装以下内置应用：

- `w7panel-higress`：Higress
- `w7panel-cloudnoauth`：CloudNoAuth

默认清单不再预装插件类型应用。维护命令为：

```bash
# 创建或更新内置预装清单
kubectl apply -f "$KO_DATA_PATH/yaml/bootstrap-installations.yaml" --server-side --prune \
  -l 'w7.cc/bootstrap-builtin=true' \
  --prune-allowlist='w7panel.w7.com/v1alpha1/BootstrapInstallation'

# 查看安装状态
kubectl get bootstrapinstallation
```

BootstrapInstallation 不维护 revision。内置声明通过 `w7.cc/bootstrap-builtin=true` 标签限定清理范围；升级脚本会删除带该标签但已不在清单中的 Installation，用户自建声明不受影响。AppGroup 可用后 Installation 进入 Ready 并停止主动协调，不定时复查；Failed 在修正无效声明、目标 AppGroup 已消失或提高 `maxRetries` 后可以重新进入安装。已达重试上限且失败 AppGroup 仍存在时保持 Failed，不再写入状态或定时重试。

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
