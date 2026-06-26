# CNB 流水线最佳实践

## 1. 用 YAML 锚点复用同文件内的配置

以 `.` 开头的 key 不会被识别为分支名，适合定义可复用片段。

```yaml
.node-env: &node-env
  docker:
    image: node:20
    volumes: [node_modules]

.install: &install
  name: install
  script: npm ci

main:
  push:
    - <<: *node-env
      stages:
        - *install
        - name: build
          script: npm run build
  pull_request:
    - <<: *node-env
      stages:
        - *install
        - name: test
          script: npm test
```

---

## 2. 按功能拆分到 `.ci/` 目录

配置超过 150 行时，按「功能 + 触发事件」拆分。每个文件只做一件事，文件名即职责。

```
.cnb.yml                    # 仅 include 列表
.ci/
├── shared-config.yml       # 公共配置
├── docker-build.yml        # Docker 镜像构建
├── pr.yml                  # PR 检查
├── push-deploy.yml         # push 部署
├── tag-release.yml         # tag 发版
└── e2e-test.yml            # E2E 测试
```

每个文件末尾声明自己的分支触发，CNB include 自动合并。公共配置通过 `!reference` 跨文件引用。

---

## 3. 善用 `failStages` / `endStages` / `allowFailure`

```yaml
- name: build-and-deploy
  stages:
    - name: build
      script: npm run build
    - name: deploy
      script: ./deploy.sh
  failStages:                    # 仅失败时
    - name: notify-failure
      image: wecom-notify
      settings: { content: "失败: $CNB_BUILD_ID" }
  endStages:                     # 始终执行
    - name: cleanup
      script: rm -rf dist
```

---

## 4. 用 `cnb:await` / `cnb:resolve` 编排多 Pipeline

```yaml
main:
  push:
    build-frontend:
      stages:
        - name: build
          script: npm run build
        - name: resolve
          type: cnb:resolve
          options: { key: frontend-ready }

    deploy:
      stages:
        - name: await frontend
          type: cnb:await
          options: { key: frontend-ready }
        - name: deploy
          script: ./deploy.sh
```

---

## 5. 给 Pipeline 和 Stage 取有意义的名称

名称直接显示在构建界面。用「动词+名词」描述操作：`install-deps`、`run-unit-tests`、`deploy-to-staging`。

---

## 6. 用自定义 Dockerfile 预装系统依赖

避免在脚本中每次执行 `apt install`、`yum install` 等系统包安装命令。每次流水线运行都会重复安装，浪费时间和资源。

```yaml
# ❌ 不推荐：每次流水线都重新安装
main:
  push:
    - docker:
        image: ubuntu:22.04
      stages:
        - name: install-deps
          script: apt-get update && apt-get install -y python3 curl jq
        - name: build
          script: python3 build.py

# ✅ 推荐：通过 Dockerfile 预装依赖
# 1. 创建 Dockerfile（如 image/Dockerfile）
# FROM ubuntu:22.04
# RUN apt-get update && apt-get install -y python3 curl jq && rm -rf /var/lib/apt/lists/*
#
# 2. 配置流水线使用 Dockerfile
# main:
#   push:
#     - docker:
#         build: image/Dockerfile
#       stages:
#         - name: build
#           script: python3 build.py
```

Dockerfile 构建的镜像会被自动缓存，仅当 Dockerfile 或依赖文件变化时才重新构建。参考：${CNB_WEB_PROTOCOL:-https}://docs.${CNB_WEB_HOST:-cnb.cool}/zh/build/build-env.md

---

## 7. 端到端示例：Node.js 项目完整 CI/CD

以下是一个 Node.js 项目的完整 `.cnb.yml`，覆盖推送测试、PR 检查、Tag 发版三种场景：

```yaml
# ── 公共配置（YAML 锚点） ─────────────────────────
.node-env: &node-env
  docker:
    image: node:20
  volumes: [node_modules]

# ── 推送：测试 + 构建 ────────────────────────────
main:
  push:
    - name: test-and-build
      <<: *node-env
      ifModify: ["src/**", "package.json"]
      stages:
        - name: install
          script: npm ci
        - name: lint
          script: npm run lint
        - name: test
          script: npm test
        - name: build
          script: npm run build
      endStages:
        - name: notify
          script: echo "Build $CNB_BUILD_ID completed"

# ── PR：检查 + 覆盖率 ────────────────────────────
"**":
  pull_request:
    - name: pr-check
      <<: *node-env
      stages:
        - name: install
          script: npm ci
        - name: lint
          script: npm run lint
        - name: test-with-coverage
          script: npm test -- --coverage
        - name: coverage-report
          type: testing:coverage
          options:
            pattern: coverage/lcov.info
            lines: 80
            diffLines: 90

# ── Tag：发版 ───────────────────────────────────
"$":
  tag_push:
    - name: release
      docker:
        build: ./Dockerfile
      services: [docker]
      stages:
        - name: build-image
          script: |
            docker build -t $CNB_DOCKER_REGISTRY/$CNB_REPO_SLUG:$CNB_BRANCH .
            docker push $CNB_DOCKER_REGISTRY/$CNB_REPO_SLUG:$CNB_BRANCH
        - name: create-release
          type: git:release
          options:
            descriptionFromFile: CHANGELOG.md
```

**要点：**
- YAML 锚点 `&node-env` 复用 Docker 配置，避免重复
- `ifModify` 跳过无关文件变更的构建
- PR 和 push 使用不同分支匹配，避免重复触发
- Tag 发版用 `docker.build` 预装构建依赖，用 `git:release` 自动发布
- `testing:coverage` 仅在 PR 事件下有增量覆盖率，此处正好用于 PR 检查