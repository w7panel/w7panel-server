# Repository Development Rules

## 本地联调

- 使用 `make dev` 启动本地服务；默认监听 `18000`，并通过 `~/.kube/218.config` 访问 218 集群。
- 需要使用其他集群时，显式传入 `KUBECONFIG_FILE=/path/to/kubeconfig`，例如 `make local-run KUBECONFIG_FILE=/path/to/kubeconfig`。
- `make dev` 的 Go 构建缓存保存在项目内已忽略的 `.w7-go-*` 目录，避免依赖用户目录的可写缓存。

## CHANGELOG 更新规则

- 每次修改代码、配置、测试或文档时，必须在同一次变更中追加更新项目根目录的 `CHANGELOG.md`。
- 如果项目根目录不存在 `CHANGELOG.md`，必须先创建，再记录本次变更。
- 只能追加新记录，不得覆盖、删除或改写已有历史记录。
- 每条记录使用 `YYYY-MM-DD` 日期，并简要写明变更内容、影响模块和验证结果。
- 提交前必须检查本次变更是否包含对应的 `CHANGELOG.md` 更新；缺少时不得提交或推送。
