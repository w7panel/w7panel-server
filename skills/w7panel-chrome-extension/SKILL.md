---
name: w7panel-chrome-extension
description: 通过 172.16.1.149 上的 Codex Chrome 扩展访问已登录的 Chrome Default 配置；需要从本机 Codex 经 SSH/MCP 使用该扩展时使用。普通共享 Chrome CDP 页面测试使用 w7panel-local-ui-test。
---

# W7Panel 远端 Codex Chrome 扩展

## 连接关系

`149` 的 Chrome `Default` 配置装有 Codex Chrome 扩展（ID `hehggadaopoacecdllhhajmbjkdcmajg`）。远端已安装社区桥接 `codex-control-chrome-mcp`，Chrome 通过 Native Messaging 启动它；本机 Codex 需要通过 SSH 启动远端 MCP stdio 服务。`172.16.1.149:9222` 是另一个 Chrome 配置的 CDP 端口，不能当作扩展连接。

## 本机连接

先确保本机能以 `afan` 身份通过 SSH 非交互登录 `172.16.1.149`。本机系统 SSH 配置曾因文件权限错误无法加载，必要时加 `-F /dev/null`。不要把 SSH 密码写入 Skill、命令参数或 Codex 配置。

在本机 Codex MCP 配置中使用 stdio 服务；远端 Node 和包路径应在版本变动后重新检查：

```toml
[mcp_servers.w7panel_chrome_extension]
command = "ssh"
args = ["-F", "/dev/null", "-T", "afan@172.16.1.149", "/home/afan/.nvm/versions/node/v24.20.0/bin/node", "/home/afan/.nvm/versions/node/v24.20.0/lib/node_modules/codex-control-chrome-mcp/bin/codex-control-chrome-mcp.js", "--stdio"]
startup_timeout_sec = 30
```

重启 Codex 会话使 MCP 配置生效。依次调用 `chrome_status`、`chrome_bridge_status`、`chrome_user_tabs`；看到 `Default` 配置中已有的标签页，才算连到了扩展。操作页面前调用 `chrome_name_session`，只认领 `chrome_user_tabs` 返回的标签页；结束时调用 `chrome_finalize_tabs`，在 `keep` 中列出需要保留的用户原有标签页。

## 远端检查与恢复

在 `149` 上运行：

```bash
codex-control-chrome-mcp status --browser chrome
```

`install.chrome.registered` 应为 `true`，并且 `sockets` 非空。若无 socket，检查 `Default` 配置中的扩展是否启用，再重新加载扩展；不要重启整个共享 Chrome。若需要重装 Native Messaging 宿主，先检查现有 `com.openai.codexextension.json`，确认可能覆盖的官方宿主和备份；获得该次修改的授权后才运行 `codex-control-chrome-mcp install-native-host --browser chrome`。

桥接来自独立社区项目，不是 OpenAI 官方组件；它可访问该 Chrome 配置中的已登录页面。不要读取或输出 cookie、密码、浏览器配置文件或页面令牌。
