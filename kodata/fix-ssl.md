# Higress SSL Passthrough 不生效修复

## 本次是怎么修复的

本次实际执行了两组操作，但作用不同：

1. 删除全部 WasmPlugin，用于排除 Wasm 插件加载失败对 Listener 更新的影响。
2. 删除 `higress-system/higress-tls-params` EnvoyFilter，解决 SSL Passthrough 的真正配置冲突。

实际命令如下：

```bash
# 第一步：备份并删除 WasmPlugin，仅用于故障隔离
kubectl get wasmplugins.extensions.higress.io -A -o yaml \
  > /tmp/wasmplugins-before-ssl-passthrough-test.yaml
kubectl delete wasmplugins.extensions.higress.io --all -A

# 第二步：备份并删除错误的 TLS EnvoyFilter，这是本次核心修复
kubectl get envoyfilter.networking.istio.io \
  -n higress-system higress-tls-params -o yaml \
  > /tmp/higress-tls-params-before-ssl-passthrough-fix.yaml
kubectl delete envoyfilter.networking.istio.io \
  -n higress-system higress-tls-params
```

删除 `higress-tls-params` 后，Envoy 的 443 Listener 从拒绝状态变为成功激活：

```text
修复前：error = No TLS certificates found for server context
修复后：error = null
```

目标 SNI 的 Active Filter Chain 变为：

```text
filter:           envoy.filters.network.tcp_proxy
upstream:         k3k-console-117057-ckm-tbgom-service-w7:443
transport_socket: none
```

最终通过 Gateway 进行 TLS 握手，客户端获得后端提供的证书，并且 HTTPS 请求返回 `HTTP/2 200`。这证明 TLS 没有在 Higress 终止，而是已透传到后端。

结论：

- **真正修复 SSL Passthrough 的操作是删除 `higress-tls-params` EnvoyFilter。**
- 删除 WasmPlugin 只是为了清除另一个独立的 Envoy Listener 拒绝因素。
- WasmPlugin 后续可以逐个恢复；不要恢复当前错误的 `higress-tls-params`。
- 因为该 EnvoyFilter 由 K3s Addon 管理，还必须从 K3s Higress 安装源清单中删除，否则以后可能被自动创建回来。

## 问题现象

Ingress 已配置 SSL Passthrough：

```yaml
metadata:
  annotations:
    higress.io/ssl-passthrough: "true"
spec:
  rules:
    - host: ss2.fan.b2.sz.w7.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: k3k-console-117057-ckm-tbgom-service-w7
                port:
                  number: 443
```

但访问域名时，Higress Gateway 没有使用新生成的 TLS 透传配置。

## 根因

Higress 2.2.3 已正确识别 `higress.io/ssl-passthrough: "true"`，并生成：

- SNI：`ss2.fan.b2.sz.w7.com`
- Network Filter：`envoy.filters.network.tcp_proxy`
- Upstream：`k3k-console-117057-ckm-tbgom-service-w7:443`

但是集群中的 `higress-system/higress-tls-params` EnvoyFilter 对 443 Listener 的所有 Filter Chain 执行 `MERGE`：

```yaml
spec:
  configPatches:
    - applyTo: FILTER_CHAIN
      match:
        context: GATEWAY
        listener:
          name: 0.0.0.0_443
      patch:
        operation: MERGE
        value:
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_params:
                  tls_minimum_protocol_version: TLSv1_1
                  tls_maximum_protocol_version: TLSv1_3
```

这会给 SSL Passthrough 的 `tcp_proxy` Filter Chain 注入一个空的 `DownstreamTlsContext`。Passthrough 本来不应该在 Gateway 终止 TLS，也不应该配置下游 TLS 证书。Envoy 因此拒绝整个新版 443 Listener：

```text
No TLS certificates found for server context
```

被拒绝后，Envoy 会继续使用旧 Listener，所以从 Kubernetes Ingress 看注解已经存在，实际流量却仍未进入新配置。

排查期间还发现过另一个独立的 Listener 拒绝错误：

```text
Unable to create Wasm plugin higress-system.ai-proxy.internal
```

WasmPlugin 加载失败也可能导致 Listener 无法更新，但它不是上述空 TLS Context 的根因。必须检查 Envoy 的最终配置状态，不能只看 Ingress 和 controller 日志。

## 立即修复

先备份并删除有问题的 EnvoyFilter：

```bash
kubectl get envoyfilter.networking.istio.io \
  -n higress-system higress-tls-params -o yaml \
  > /tmp/higress-tls-params-before-ssl-passthrough-fix.yaml

kubectl delete envoyfilter.networking.istio.io \
  -n higress-system higress-tls-params
```

删除该 EnvoyFilter 不会删除 HTTPS 证书或 Ingress 路由。它只会取消这条全局 TLS 版本覆盖，HTTPS Listener 将使用 Higress/Envoy 的默认 TLS 参数。

如果同时存在 Wasm 加载错误，应先备份，再停用或删除故障插件：

```bash
kubectl get wasmplugins.extensions.higress.io -A -o yaml \
  > /tmp/wasmplugins-before-ssl-passthrough-test.yaml

kubectl delete wasmplugins.extensions.higress.io --all -A
```

删除全部 WasmPlugin 适合故障隔离，不建议作为长期方案。确认具体故障插件后，应只停用或修复对应插件，再逐个恢复其他插件：

```bash
kubectl apply -f /tmp/wasmplugins-before-ssl-passthrough-test.yaml
```

恢复前需要确认 `ai-proxy.internal` 的 Wasm 文件可下载、可读取且能够成功初始化，否则可能再次导致 Listener 被拒绝。

## 永久修复

当前 `higress-tls-params` 带有以下注解，说明它由 K3s Addon 管理：

```yaml
objectset.rio.cattle.io/owner-gvk: k3s.cattle.io/v1, Kind=Addon
objectset.rio.cattle.io/owner-name: higress
objectset.rio.cattle.io/owner-namespace: kube-system
```

只执行 `kubectl delete` 可能在 K3s 重启、Addon 刷新或重新安装 Higress 后被重新创建。需要修改 K3s Server 节点上的 Higress Addon 源清单，删除 `higress-tls-params` 资源，然后再删除集群内现有资源。

常见清单目录为：

```text
/var/lib/rancher/k3s/server/manifests/
```

永久处理建议：

1. 从 Higress Addon/安装清单中移除当前全局 `higress-tls-params` EnvoyFilter。
2. 不要通过 `applyTo: FILTER_CHAIN` 给整个 `0.0.0.0_443` Listener 注入 `transport_socket`。
3. 如果必须限制 TLS 版本，应在生成 TLS termination Server/DownstreamTlsContext 的控制面代码或受支持的 TLS 配置入口中设置参数。
4. 如果只能使用 EnvoyFilter，应确保补丁只修改已经存在 TLS termination socket 的 HTTP Filter Chain，不能命中 `envoy.filters.network.tcp_proxy` Passthrough Chain。
5. 当前集群版本中，给上述 `FILTER_CHAIN` 匹配增加嵌套 `http_connection_manager` 条件仍然命中了 Passthrough Chain，因此不要把该写法当作有效修复。

## 验证方法

### 1. 检查 Ingress 和后端

```bash
kubectl get ingress -n k3k-console-117057 \
  ing-ecbztclx-default-console-117057-696e672d6563627a74636-a8983 -o yaml

kubectl get svc -n k3k-console-117057 \
  k3k-console-117057-ckm-tbgom-service-w7 -o yaml

kubectl get endpointslice -n k3k-console-117057 \
  -l kubernetes.io/service-name=k3k-console-117057-ckm-tbgom-service-w7
```

应确认：

- 注解值为字符串 `"true"`。
- Ingress 包含 `/` 根路径；SSL Passthrough 不按 HTTP 子路径分流。
- Service 443 端口存在。
- EndpointSlice 中至少有一个 `ready: true` 的 443 Endpoint。

### 2. 读取 Envoy Config Dump

```bash
POD=$(kubectl get pod -n higress-system \
  -l app=higress-gateway \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n higress-system "$POD" -- \
  pilot-agent request GET config_dump \
  > /tmp/higress-config-dump.json
```

检查 443 Listener 是否还有拒绝错误：

```bash
jq -r '
  .configs[]
  | select(.["@type"] | contains("ListenersConfigDump"))
  | .dynamic_listeners[]
  | select(.name == "0.0.0.0_443")
  | {
      active_updated: .active_state.last_updated,
      error: .error_state.details
    }
' /tmp/higress-config-dump.json
```

正常结果中 `error` 应为 `null`。

检查目标域名的 Active Filter Chain：

```bash
jq -r '
  .configs[]
  | select(.["@type"] | contains("ListenersConfigDump"))
  | .dynamic_listeners[]
  | select(.name == "0.0.0.0_443")
  | .active_state.listener.filter_chains[]
  | select(.filter_chain_match.server_names? | index("ss2.fan.b2.sz.w7.com"))
  | {
      filters: [.filters[].name],
      cluster: .filters[0].typed_config.cluster,
      transport_socket: (.transport_socket.name // "none")
    }
' /tmp/higress-config-dump.json
```

预期结果：

```json
{
  "filters": [
    "envoy.filters.network.tcp_proxy"
  ],
  "cluster": "outbound|443||k3k-console-117057-ckm-tbgom-service-w7.k3k-console-117057.svc.cluster.local",
  "transport_socket": "none"
}
```

`transport_socket: "none"` 是关键：表示 Higress 没有终止客户端 TLS，而是根据 ClientHello 中的 SNI 将原始 TLS 流量转发到后端。

### 3. 验证 TLS 握手

如果操作机器不能直接访问 LoadBalancer IP，可以临时使用端口转发：

```bash
kubectl port-forward -n higress-system svc/higress-gateway 14443:443
```

另开终端执行：

```bash
openssl s_client \
  -connect 127.0.0.1:14443 \
  -servername ss2.fan.b2.sz.w7.com </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -serial -fingerprint -sha256

curl -skI \
  --resolve ss2.fan.b2.sz.w7.com:14443:127.0.0.1 \
  https://ss2.fan.b2.sz.w7.com:14443/
```

本次修复后的实际结果为：

```text
subject=CN=ss2.fan.b2.sz.w7.com
issuer=C=US, O=Let's Encrypt, CN=YR1
HTTP/2 200
```

这说明客户端拿到的是后端服务提供的证书，SSL Passthrough 已生效。
