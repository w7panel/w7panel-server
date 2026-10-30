# 专属菜单策略（TAPD 1016217）

在 Server 所在命名空间创建 `w7panel-permission-exclusives` ConfigMap，`data.policy` 为 JSON。未创建时策略为空；创建后格式错误会拒绝受保护的请求。此 ConfigMap 由集群管理员在面板外维护，面板代理拒绝修改它。

```json
{"rules":[{"menuRule":"cluster/example","menuKeys":["cluster-example"],"exclusiveGroups":["ops","security"],"apiRules":[{"path":"/panel-api/v1/example/*","method":["get","update"]}]}]}
```

- `menuRule` 使用 Permission CRD `spec.menuRules` 的路径值；`menuKeys` 可列出旧版保存的菜单键。
- `exclusiveGroups` 列出允许该菜单及关联 API 的实际用户组，可有多个。没有预设专属菜单。
- `GET /panel-api/v1/auth/permission-exclusives` 仅返回有效规则和当前认证用户组，供 UI 灰显及路由守卫使用。
- `apiRules` 使用 Permission API 的方法名 `get`、`list`、`create`、`update`、`patch`、`delete`；路径中的 `*` 与现有 Permission 路径匹配语义相同。不同规则项之间重叠的路径会被拒绝，同一规则项内 `get` 与 `list` 视为重复的 GET 方法。
- 经面板 `/k8s-proxy` 写入 Permission/User CRD 会校验最终对象。直接持有 Kubernetes 写权限的管理员仍可写入 CRD；Server 的菜单下发和 API 访问会按策略再次限制。若需阻止此类对象落库，还需 Kubernetes Admission 约束。
