# Repository Development Rules

## 发布顺序

- 每次向 218 部署测试版本，都先在相邻的 `w7panel-ui` 目录执行 `./build.sh`，确认前端产物已写入本仓库的 `kodata`。
- 然后在本仓库指定本次镜像标签并依次推送、发布：

  ```sh
  export IMAGE_TAG=v1.1.96.xxx
  make ko-push && ./pub.sh
  ```

- 每次使用新的 `IMAGE_TAG`；`make ko-push` 只打包现有 `kodata`，不会自动构建前端。`pub.sh` 是被 Git 忽略的本机脚本，执行前确认它将 `IMAGE_TAG` 传给 Helm 的 `image.tag`。发布后核对 Helm 实际使用的镜像标签与 `IMAGE_TAG` 一致。

## 本地联调

- 使用 `make dev` 启动本地服务；默认监听 `18000`，并通过 `~/.kube/218.config` 访问 218 集群。
- 需要使用其他集群时，显式传入 `KUBECONFIG_FILE=/path/to/kubeconfig`，例如 `make local-run KUBECONFIG_FILE=/path/to/kubeconfig`。
- `make dev` 的 Go 构建缓存保存在项目内已忽略的 `.w7-go-*` 目录，避免依赖用户目录的可写缓存。

## Go 构建与测试缓存

- 必须使用 `make test`（可附加 `TEST_PACKAGES=...`）运行 Go 测试；不得直接运行 `go test`。
- 必须使用 `make ko-build`、`make ko-push` 或 `make image` 构建镜像；不得直接运行 `ko build`。
- 对未被 Makefile 覆盖的 Go 构建，必须复用 Makefile 定义的项目内 `.w7-go-cache`、`.w7-go-modcache`、`.w7-gopath`、`.w7-go-tmp`；不得将编译缓存或 Go 临时工作目录写入用户目录或 `/tmp`。

## CHANGELOG 更新规则

- 每次修改代码、配置、测试或文档时，必须在同一次变更中追加更新项目根目录的 `CHANGELOG.md`。
- 如果项目根目录不存在 `CHANGELOG.md`，必须先创建，再记录本次变更。
- 只能追加新记录，不得覆盖、删除或改写已有历史记录。
- 每条记录使用 `YYYY-MM-DD` 日期，并简要写明变更内容、影响模块和验证结果。
- 提交前必须检查本次变更是否包含对应的 `CHANGELOG.md` 更新；缺少时不得提交或推送。




<!-- code-review-graph MCP tools -->
## MCP Tools: code-review-graph

**IMPORTANT: This project has a knowledge graph. ALWAYS use the
code-review-graph MCP tools BEFORE using Grep/Glob/Read to explore
the codebase.** The graph is faster, cheaper (fewer tokens), and gives
you structural context (callers, dependents, test coverage) that file
scanning cannot.

### When to use graph tools FIRST

- **Exploring code**: `semantic_search_nodes_tool` or `query_graph_tool` instead of Grep
- **Understanding impact**: `get_impact_radius_tool` instead of manually tracing imports
- **Code review**: `detect_changes_tool` + `get_review_context_tool` instead of reading entire files
- **Finding relationships**: `query_graph_tool` with callers_of/callees_of/imports_of/tests_for
- **Architecture questions**: `get_architecture_overview_tool` + `list_communities_tool`

Fall back to Grep/Glob/Read **only** when the graph doesn't cover what you need.

### Key Tools

| Tool | Use when |
| ------ | ---------- |
| `detect_changes_tool` | Reviewing code changes — gives risk-scored analysis |
| `get_review_context_tool` | Need source snippets for review — token-efficient |
| `get_impact_radius_tool` | Understanding blast radius of a change |
| `get_affected_flows_tool` | Finding which execution paths are impacted |
| `query_graph_tool` | Tracing callers, callees, imports, tests, dependencies |
| `semantic_search_nodes_tool` | Finding functions/classes by name or keyword |
| `get_architecture_overview_tool` | Understanding high-level codebase structure |
| `refactor_tool` | Planning renames, finding dead code |

### Workflow

1. The graph auto-updates on file changes (via hooks).
2. Use `detect_changes_tool` for code review.
3. Use `get_affected_flows_tool` to understand impact.
4. Use `query_graph_tool` pattern="tests_for" to check coverage.

## Worktree 分支

- 任务 worktree 和对应的 `worktree/*` 分支仅在本地使用，不得推送到远端。需要共享任务改动时，先按 worktree 合并流程合入主项目，再推送主分支。

## Worktree 合并后清理

- 任务 worktree 的改动合并到主分支并完成提交后，确认主项目当前提交已包含全部任务改动，再在主项目目录执行 `$worktree-end 任务名` 删除该 worktree 和任务分支。如仍有未合并差异，停止清理；不得用 `--force` 跳过检查。
