---
name: w7panel-local-ui-test
description: 在本地用 w7panel-server 和 w7panel-ui 验证 W7Panel 页面；需要启动服务端、挂载指定 kubeconfig、运行 Vite，并通过共享 Chrome/CDP 做页面检查时使用。
---

# W7Panel 本地 UI 测试

## 目标

在开发机上启动服务端和 UI，使用测试集群 kubeconfig 验证真实接口和页面。默认服务端端口为 `18000`，UI 端口为 `8011`，浏览器必须访问开发机 LAN 地址 `http://172.16.1.18:8011/`。

## 启动服务端

优先直接运行源码（无需每次构建镜像）：

```bash
cd /home/afan/workspace/ai/w7panel-server
GOSUMDB=off \
GOPATH=/home/afan/workspace/ai/.w7-gopath \
GOMODCACHE=/home/afan/workspace/ai/.w7-go-modcache \
GOCACHE=/home/afan/workspace/ai/.w7-go-cache \
KUBECONFIG=/home/afan/.kube/218.config \
W7PANEL_HTTP_SERVER_PORT=18000 \
go run . server:start
```

如果必须验证镜像入口，使用仓库 Makefile，并显式传入 kubeconfig：

```bash
make docker-run LOCAL_IMAGE=w7panel-local IMAGE_TAG=<tag> \
  KUBECONFIG_FILE=/home/afan/.kube/218.config \
  CONTAINER_NAME=w7panel-local-test HOST_PORT=18000 CONTAINER_PORT=18000
```

不要把 kubeconfig、token 或 Secret 写入 Skill、日志或提交内容。启动后检查：

```bash
curl -i --max-time 10 http://172.16.1.18:18000/health
docker logs --tail 40 w7panel-local-test   # 仅镜像模式
```

若 Go 报工具链版本或下载错误，先检查 `go version`、`go env GOTOOLCHAIN GOPATH GOMODCACHE` 和磁盘空间；不要修改并提交 `go.mod` 来绕过版本问题。

## 启动 UI

```bash
cd /home/afan/workspace/ai/w7panel-ui
npm run dev -- --host 0.0.0.0 --port 8011
```

确认 Vite 输出 `Network: http://172.16.1.18:8011/`。开发代理配置默认来自 `config/vite.config.dev.ts`；需要本地后端时，将代理目标临时指向 `http://172.16.1.18:18000`，测试后恢复且不要提交临时配置。

## 浏览器检查

共享 Chrome CDP 固定为 `http://172.16.1.149:9222`：

```bash
curl -s --max-time 5 http://172.16.1.149:9222/json/version
BU_CDP_URL=http://172.16.1.149:9222 browser-use <<'PY'
new_tab('http://172.16.1.18:8011/')
wait(2)
print(page_info())
print(capture_screenshot())
PY
```

按“截图 -> 定位可见控件 -> 坐标点击 -> 等待 -> 截图”的顺序验证路由、选中菜单和页面标识文本。CDP 端点可访问但 browser-use 报 `Connection lost` 时，记录为环境阻塞并使用 `scripts/cdp_click_test.mjs`（若测试场景支持）或仅报告 HTTP 健康检查结果；不要反复重启共享 Chrome。

## 清理

停止本次启动的 Vite 进程；镜像模式停止容器（容器使用 `--rm`）：

```bash
docker stop w7panel-local-test
```

不要停止共享 Chrome 或删除用户已有容器、镜像和缓存。
