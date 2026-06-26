---
name: cnb-pipeline
description: 编写、修改、审查 .cnb.yml 流水线配置，诊断构建失败原因，优化构建性能；覆盖 CI/CD、pipeline 语法、触发事件、环境变量、内置任务、build error、build slow。
supports: cnb
---

# CNB 流水线配置与诊断

> 本文档 URL 中的 `${CNB_WEB_PROTOCOL:-https}` 和 `${CNB_WEB_HOST:-cnb.cool}` 为环境变量，使用前先 `echo` 获取实际值再拼接。

## 模式判定

根据用户意图自动选择工作模式：

- **配置模式**（写/改流水线）→ 走下方「配置工作流程」
- **诊断模式**（失败/报错/慢/优化）→ 走下方「诊断工作流程」

## 配置工作流程

1. **了解需求** -- 明确触发分支、事件、构建语言/环境、构建步骤、特殊需求。信息充足可直接生成。
2. **查看现有配置** -- 修改场景下先读取 `.cnb.yml` 和 `.ci/` 目录。
3. **按需加载文档** -- 遇到不确定的语法细节时，读取本 skill 的 `references/` 子目录下对应参考文件；也可 `echo` 获取环境变量拼接在线文档 URL 用 WebFetch 加载。
4. **生成配置** -- 按下方「结构骨架」生成完整可运行的配置。
5. **校验（必须）** -- 每次生成/修改 `.cnb.yml` 或 `.cnb/web_trigger.yml` 后必须校验通过才能展示给用户。
6. **解释配置** -- 简要说明关键部分。

### 校验命令

> 校验脚本位于本 skill 的 `validator/` 子目录。执行时需用本 skill base directory 的绝对路径（加载 skill 时系统会提示该路径）。

```bash
cd ${SKILL_BASE}/validator && [ -d node_modules ] || npm install
node ${SKILL_BASE}/validator/validate.js <yml-file-path>
```

其中 `${SKILL_BASE}` 替换为本 skill 的 base directory 绝对路径，`<yml-file-path>` 为待校验文件的绝对路径。脚本按文件名自动识别校验类型，无需额外参数：

- `.cnb.yml`（流水线配置）→ `YAML 语法` → `语义校验` → `Schema`，三项均通过才算有效。
- `.cnb/web_trigger.yml`（Web 触发器配置）→ `YAML 语法` → `目录约束`（必须位于 `.cnb/` 下）→ `Schema`，不走 `.cnb.yml` 的语义规则；该文件为可选配置，仓库未配置时校验自动跳过。

`--refresh` 可强制更新 Schema 缓存。

### 校验失败处理

校验不通过时按以下流程修复：

1. **YAML 语法错误** → 检查缩进（必须空格）、引号、特殊字符转义
2. **语义校验错误** → 根据错误信息定位问题，常见修复：
   - `CNB_ 前缀变量` → 检查拼写，参考 `references/env-variables.md` 内置变量列表
   - `内置任务事件限制` → 将任务移至支持的事件下，参考 `references/builtin-tasks.md` 事件支持表
   - `仓库级事件位置` → 将 `issue.*`/`tag_push` 等移至 `$` 兜底分支下
   - `crontab 事件位置` → 将 `crontab` 移至具体分支名下
3. **Schema 校验错误** → 检查字段名拼写、类型、必填项，参考 `references/syntax-reference.md`

修复后重新执行校验命令，直到全部通过。

## 诊断工作流程

> 详细流程见 `references/diagnose-guide.md`，依赖 [cnb-api] skill。

1. **确定构建 sn**（可选）-- 默认不传，CLI 自动解析；需指定时从 `cnb pulls check-status` 对应检查项的 `target_url` 末段取（勿用 `context` 字段）。
2. **获取数据**：
   - 失败诊断：`cnb pulls get-ci-logs`（自动定位失败构建；也可加 `--sn` 指定）
   - 性能优化：通过 `cnb build --help` / `cnb pulls --help` 探索可用命令，获取 Stage 耗时与慢 Stage 日志
3. **分析并输出报告** -- 判定失败类型或耗时瓶颈，给出修复/优化建议。配置相关问题结合 references 分析。

---

## 结构骨架

配置层级：**分支 → 事件 → Pipeline → Stage → Job**

```yaml
<branch>:                          # main / "feature/*" / "$"(兜底)
  <event>:                         # push / pull_request / tag_push / ...
    - name: <pipeline-name>
      runner: { cpus: 2 }
      docker:
        image: node:20             # 或 build (Dockerfile) 或 devcontainer
      services: [docker]           # Docker-in-Docker
      env: { KEY: value }
      imports: [./secrets.yml]
      stages:
        - name: step1
          script: echo hello       # 脚本任务
        - name: step2
          type: cnb:trigger        # 内置任务
          options: { ... }
        - name: step3
          image: plugins/docker    # 插件任务
          settings: { ... }
      failStages: [...]            # 仅失败时执行
      endStages: [...]             # 始终执行
```

> **详细语法**：完整触发事件列表、Pipeline/Stage/Job 全部字段、变量替换规则、include/!reference、数据卷缓存等见 `references/syntax-reference.md`。
> **内置变量**：常用变量速查见下方，完整 82 个变量列表见 `references/env-variables.md`。
> **内置任务**：所有内置任务类型、参数、支持的事件见 `references/builtin-tasks.md`。

### 常用内置变量（速查）

| 变量 | 说明 |
|------|------|
| `CNB_BRANCH` | 分支/Tag 名 |
| `CNB_COMMIT` / `CNB_COMMIT_SHORT` | Commit SHA |
| `CNB_REPO_SLUG` | 仓库路径 |
| `CNB_BUILD_ID` | 构建 ID |
| `CNB_TOKEN` | 构建凭证（PR 事件权限受限） |
| `CNB_EVENT` | 事件名 |
| `CNB_PULL_REQUEST_IID` | PR 编号 |

> **重要**：`CNB_` 前缀为系统保留，禁止自定义同名变量。引用非内置 `CNB_` 变量通常是拼写错误。

---

## 详细参考文档

| 文件 | 何时读取 |
|------|----------|
| `references/syntax-reference.md` | 需要完整事件列表、Pipeline/Stage/Job 全部字段、include/!reference、数据卷、部署配置时 |
| `references/builtin-tasks.md` | 需要内置任务参数、支持事件列表时 |
| `references/env-variables.md` | 需要完整内置变量列表、变量声明/导出/传递规则时 |
| `references/best-practices.md` | 需要配置模式参考、Dockerfile 预装依赖、端到端示例时 |
| `references/diagnose-guide.md` | 诊断模式：失败类型判定、性能优化分析时 |

---

## 校验规则

除 YAML 语法和 Schema 校验外，语义校验会额外检查以下规则：

### 错误（校验不通过）

1. **CNB_ 前缀变量检查** — 禁止自定义/引用非内置 `CNB_` 变量，通常是拼写错误
2. **内置任务事件限制** — 部分内置任务仅支持特定事件（如 `git:auto-merge` 仅 `pull_request.mergeable`），详见 `references/builtin-tasks.md`
3. **仓库级事件位置** — `issue.*`/`tag_push`/`tag_deploy.*`/`vscode`/`auto_tag` 只能放在 `$` 兜底分支下
4. **crontab 事件位置** — 必须放在具体分支名下

### 警告（建议优化）

1. **系统依赖手动安装** — 检测到 `apt install`/`yum install`/`apk add` 等时建议改用 `docker.build` Dockerfile 预装
2. **web_trigger / api_trigger 位置** — 建议放在 `$` 兜底分支下

---

## 注意事项

1. **YAML 缩进**用空格，不用 Tab
2. **分支名含特殊字符**需引号包裹：`"feature/*"`
3. **并发模型**：同事件多 Pipeline 并发；Pipeline 内 Stage 顺序；Stage 内 jobs 数组串行、对象并行
4. **PR 安全限制**：PR 类事件 `CNB_TOKEN` 权限受限，敏感操作放 `push` / `pull_request.target` / `tag_push`
5. **YAML 锚点仅限单文件**：跨文件用 `!reference`（只能引用值，不能合并展开）
6. **`!reference` 引用键名必须全局唯一**：跨文件共享时加文件/模块前缀避免冲突
7. **变量值上限 100KiB**，变量名只能含字母/数字/下划线且不能数字开头
