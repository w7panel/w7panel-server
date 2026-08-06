# CHANGELOG

## 2026-08-05

- 修复顶部微应用角色数量判断：只统计面板支持的 `founder`、`super`、`normal` Binding，`zpk-market` 等功能菜单分组不再把单角色应用提升到顶部菜单。
- 影响模块：面板角色定义、MicroApp 顶部列表接口。
- 验证：`go test ./common/service/k8s/microapp -run TestPanelRoleBindingCount -count=1` 与 `go test ./common/service/k8s/permission -run TestIsPanelRole -count=1` 通过。

## 2026-08-05

- 新增项目开发规则：此后每次修改代码、配置、测试或文档，都必须追加更新本文件。
- 影响模块：项目开发与提交流程。
- 验证：已确认 `AGENTS.md` 包含追加写入和提交前检查要求。

## 2026-08-05

- 统一 MicroApp 菜单权限过滤：founder 展示全部 Binding 菜单，其他角色沿用现有角色过滤规则，不在面板中硬编码具体菜单分组名称。
- 影响模块：MicroApp 列表、详情及根 MicroApp 同步过滤。
- 验证：`go test ./common/service/k8s/microapp` 通过。

## 2026-08-05

- 删除 AppGroup 外部服务字段及其 ZPK 响应映射、安装转换、生成客户端和测试代码，服务中心入口统一由 MicroApp Binding 提供。
- 影响模块：AppGroup CRD、ZPK 安装与 ManifestPackage、Kubernetes 生成代码、后端文档。
- 验证：`go build ./...`、AppGroup 生成客户端相关包测试及 `TestSyncAppGroupZpkURL` 通过。
