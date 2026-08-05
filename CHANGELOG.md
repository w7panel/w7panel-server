# CHANGELOG

## 2026-08-05

- 新增项目开发规则：此后每次修改代码、配置、测试或文档，都必须追加更新本文件。
- 影响模块：项目开发与提交流程。
- 验证：已确认 `AGENTS.md` 包含追加写入和提交前检查要求。

## 2026-08-05（BootstrapInstallation）

- 移除无实际协调作用的 `BootstrapInstallation.spec.revision`，Operation ID 改为基于资源 UID 稳定生成。
- 为内置 BootstrapInstallation 增加专用标签，并在升级脚本中通过标签和资源类型 allowlist 安全清理已从清单移除的声明。
- 影响模块：BootstrapInstallation API、CRD、内置清单、升级脚本、测试与开发文档。
- 验证：API/CRD/内置清单回归测试、Bootstrap 核心测试（排除工作区中既有的未完成 Token 测试）、升级脚本语法检查及两套 Helm Chart lint 均通过；完整 Bootstrap 包测试仍被既有 `clusterTokenFromRESTConfig` 缺失阻断。
