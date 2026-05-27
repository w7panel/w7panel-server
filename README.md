# W7Panel 后端

基于 Go + Gin 的 Kubernetes 云原生应用管理平台后端服务。

## 技术栈

- **Go 1.25** - 编程语言
- **Gin** - Web 框架
- **w7-rangine-go** - 应用框架
- **Kubernetes** - 容器编排
- **SQLite** - 嵌入式数据库

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
├── database/                    # 数据库生成和模型配置
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

## 快速开始

### 环境要求

- Go 1.25+
- Node.js 18+ (用于前端构建)
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
| `W7PANEL_OFFLINE_HTTP_SERVER_PORT` | 8080 | HTTP 端口 |

## 主要功能

- **WebDAV 文件管理** - 容器内文件在线管理
- **压缩/解压** - 支持 zip, tar, tar.gz, tar.xz
- **权限管理** - chmod, chown 操作
- **应用部署** - Helm, Docker Compose, YAML
- **集群管理** - 节点、资源对象管理

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
