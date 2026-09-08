# ZpkInstall 一次性安装任务

HTTP `PUT /panel-api/v1/zpk/install` 保持原有参数、权限和同步响应；新增 CRD 通过同一安装服务执行。升级包含新 CRD 的 Server 后，按现有 CRD 初始化流程安装 schema，并由 Controller Manager 自动监听任务。必须先确保 CRD 已建立，再启动监听。

## 创建任务

在需要安装应用的集群创建 CR，主集群不会把任务转发到远程 CKM。以下 URL 和 identifie 需替换为实际制品信息；installOptions 与原 install API 的业务字段一致，可先通过现有 config API 获取。

```yaml
apiVersion: w7panel.w7.com/v1alpha1
kind: ZpkInstall
metadata:
  name: install-my-app-001
  namespace: default
spec:
  repoUrl: https://example.com/package
  releaseName: my-app
  installOptions:
    - identifie: my-app
      replicas: 1
      envkv:
        - name: MODE
          value: production
  ingressHost: app.example.com
  ingressClass: higress
```

`spec.namespace` 缺省使用 CR 的 namespace；制品声明的 Helm namespace、单实例名称及控制台 PreInstall 返回的 releaseName 仍按原安装逻辑生效，最终结果查看 status。

支持 ingressSeletorName（保留原 API 拼写）、ingressForceHttps、panelUrl、isTrandition、zipUrl、reinstall 等业务参数。CRD 不接收 clusterId 或执行 Kubernetes token、ServiceAccount、installId、HTTP 构建回调等运行时参数。修改 spec 会被拒绝，升级或重试需使用新的 metadata.name 创建任务。

## 私有制品凭据

第三方凭据不内嵌到 spec，使用 CR 同命名空间 Secret；引用不存在、key 缺失或为空时任务失败，即使 SecretKeySelector.optional=true 也不跳过认证。

```yaml
spec:
  # 与上例其他必填参数一起使用
  thirdpartyCDTokenRef:
    name: zpk-repository-credentials
    key: thirdpartyCDToken
  # 仅在制品仓库需要额外面板身份时设置
  panelTokenRef:
    name: zpk-repository-credentials
    key: panelToken
```

Secret 中 panelToken 只传给制品请求，不决定安装目标或 Kubernetes 权限；实际执行身份始终是本集群 Server。Secret 应由管理员通过安全渠道创建，避免把真实 token 提交到 Git。安装参数中的环境变量、镜像仓库密码也应谨慎保存，优先使用现有 Secret 引用能力。

## 权限

该任务可以安装集群级 Helm 资源、使用 Server 权限和运行安装脚本，因此创建权限等价于授权执行高权限安装。默认仅 founder 和系统 Server 账号可写；不增加 normal、super 或 api 的写权限。自定义通配 RBAC 的管理员需要自行审计既有授权。

内置 Server ClusterRole 已有全部资源权限，不额外扩大授权。使用自定义 Server Role 时，需在既有安装权限之外增加：

```yaml
- apiGroups: ["w7panel.w7.com"]
  resources: ["zpkinstalls"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["w7panel.w7.com"]
  resources: ["zpkinstalls/status"]
  verbs: ["get", "update", "patch"]
```

只应向可信自动化账号授予 zpkinstalls 的 create/get/list/watch/delete 权限；不要授予提交方 status 写权限，以免绕过执行保护。

## 状态与中断

```sh
kubectl get zpkinstalls -n default
kubectl get zpkinstall install-my-app-001 -n default -o yaml
```

- Pending/空状态：尚未领取。
- Running：已经持久化执行 ID 和 installId，开始执行；每 30 秒更新心跳。
- Succeeded：与 HTTP install 的成功含义一致，安装动作已提交，不代表所有 Job 或工作负载就绪。
- Failed：安装服务失败，或凭据不可用；status.reason 提供分类，message 不回显原始制品错误中的敏感内容。
- Unknown：心跳超过 120 秒未更新，或执行完成结果未能持久化且后续心跳超时；不自动重放。

任务最多领取执行一次，但 Kubernetes 资源、外部安装绑定和 status 不属于同一事务，因此不承诺所有外部副作用恰好一次。心跳超时也不能证明旧进程已停止，Unknown 后先检查 AppGroup、Job、Helm release 和原执行进程，再决定是否新建任务。

删除任务不会卸载应用，也不是取消正在执行安装的可靠方式。任务不作为安装产物的 OwnerReference，不添加卸载 finalizer。需要卸载时继续使用原卸载流程。

CRD 不生成依赖 HTTP Request.Host 的构建成功回调；现有 BuildImageSuccess 方法没有执行业务。HTTP 入口仍保留其原有回调参数行为。
