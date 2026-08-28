KO ?= ko
LOCAL_IMAGE ?= w7panel
IMAGE_TAG ?= $(shell branch=$$(git branch --show-current 2>/dev/null); [ -n "$$branch" ] || branch=local; printf '%s' "$$branch" | tr '/' '-')
PLATFORM ?= linux/amd64
KO_DEFAULTBASEIMAGE ?= ccr.ccs.tencentyun.com/afan-public/ubuntu:24.04-offlineui
UI_DIR ?= $(CURDIR)/../w7panel-ui
UI_BUILD_SCRIPT ?= $(UI_DIR)/build.sh
CONTAINER_NAME ?= w7panel-local
HOST_PORT ?= 18000
CONTAINER_PORT ?= 18000
KUBECONFIG_FILE ?= $(HOME)/.kube/config
OIDC_ISSUER ?= http://172.16.1.18:18000/panel-api/v1/oidc
LOCAL_PORT ?= 18000
LOCAL_GO_CACHE ?= $(CURDIR)/.w7-go-cache
LOCAL_GO_MODCACHE ?= $(CURDIR)/.w7-go-modcache
LOCAL_GOPATH ?= $(CURDIR)/.w7-gopath
LOCAL_GO_TMP ?= $(CURDIR)/.w7-go-tmp
GO_TOOLCHAIN_ROOT ?= /home/afan/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64
GO_BIN ?= $(if $(wildcard $(GO_TOOLCHAIN_ROOT)/bin/go),$(GO_TOOLCHAIN_ROOT)/bin/go,go)
DOCKER_RUN_ARGS ?=

.PHONY: help frontend image ko-build docker-run local-run

help:
	@echo "本地镜像构建："
	@echo "  make image"
	@echo "  make image LOCAL_IMAGE=w7panel IMAGE_TAG=dev PLATFORM=linux/arm64"
	@echo "  make frontend UI_DIR=../w7panel-ui"
	@echo "本地容器运行："
	@echo "  make docker-run"
	@echo "  make docker-run HOST_PORT=18000 OIDC_ISSUER=http://172.16.1.18:18000/panel-api/v1/oidc"
	@echo "本地源码运行："
	@echo "  make local-run KUBECONFIG_FILE=/home/afan/.kube/218.config"

# 调用相邻 w7panel-ui 的构建脚本，将前端产物放入 ko 自动打包的 kodata 目录。
frontend:
	@test -f "$(UI_BUILD_SCRIPT)" || { echo "前端构建脚本不存在：$(UI_BUILD_SCRIPT)"; exit 1; }
	cd "$(UI_DIR)" && sh "$(UI_BUILD_SCRIPT)"
	mkdir -p "$(CURDIR)/kodata/assets"
	cp "$(CURDIR)/kodata/logo.png" "$(CURDIR)/kodata/assets/logo.png"
	cp "$(CURDIR)/kodata/index.html" "$(CURDIR)/kodata/panel.html"
	@echo "前端构建完成：$(CURDIR)/kodata/index.html"

# 完整镜像先构建前端，再执行 ko；ko-build 可复用现有 kodata 直接重试镜像构建。
image: frontend ko-build

# 参考 .cnb.yml 的 ko build 参数，将包含前端资源的单平台镜像加载到本机 Docker。
ko-build:
	@command -v $(KO) >/dev/null 2>&1 || { echo "未找到 ko，请先安装：go install github.com/google/ko@latest"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "未找到 docker，请先安装并启动 Docker"; exit 1; }
	KO_DOCKER_REPO="$(LOCAL_IMAGE)" \
	KO_DEFAULTBASEIMAGE="$(KO_DEFAULTBASEIMAGE)" \
	$(KO) build \
		--local \
		--bare \
		--tags="$(IMAGE_TAG)" \
		--tag-only \
		--sbom=none \
		--platform="$(PLATFORM)" \
		.
	@echo "本地镜像构建完成：$(LOCAL_IMAGE):$(IMAGE_TAG)"

docker-run:
	@command -v docker >/dev/null 2>&1 || { echo "未找到 docker，请先安装并启动 Docker"; exit 1; }
	@test -f "$(KUBECONFIG_FILE)" || { echo "Kubeconfig 不存在：$(KUBECONFIG_FILE)"; exit 1; }
	docker run --rm -d \
		--name "$(CONTAINER_NAME)" \
		-p "$(HOST_PORT):$(CONTAINER_PORT)" \
		-e "KUBECONFIG=/root/.kube/config" \
		-e "OIDC_ISSUER=$(OIDC_ISSUER)" \
		-e "W7PANEL_HTTP_SERVER_PORT=$(CONTAINER_PORT)" \
		-v "$(KUBECONFIG_FILE):/root/.kube/config:ro" \
		$(DOCKER_RUN_ARGS) \
		"$(LOCAL_IMAGE):$(IMAGE_TAG)" \
		server:start
	@echo "容器已启动：$(CONTAINER_NAME)，访问 http://127.0.0.1:$(HOST_PORT)"

# 直接运行源码，适合本地联调，避免每次构建镜像。
local-run:
	@test -f "$(KUBECONFIG_FILE)" || { echo "Kubeconfig 不存在：$(KUBECONFIG_FILE)"; exit 1; }
	@mkdir -p "$(LOCAL_GO_TMP)"
	GOSUMDB=off \
	GOPATH="$(LOCAL_GOPATH)" \
	GOMODCACHE="$(LOCAL_GO_MODCACHE)" \
	GOCACHE="$(LOCAL_GO_CACHE)" \
	GOTMPDIR="$(LOCAL_GO_TMP)" \
	GOROOT="$(if $(wildcard $(GO_TOOLCHAIN_ROOT)/bin/go),$(GO_TOOLCHAIN_ROOT),)" \
	KUBECONFIG="$(KUBECONFIG_FILE)" \
	W7PANEL_HTTP_SERVER_PORT="$(LOCAL_PORT)" \
	"$(GO_BIN)" run . server:start
