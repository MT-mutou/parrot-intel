# 鹦鹉情报站 ParrotIntel —— 构建/发布编排
# Windows 无 make 时直接用 scripts/release.sh（功能等同 release 目标）

VER ?= 2.2.0

.PHONY: help dev-server dev-web build-server build-web build release clean

help:
	@echo "make dev-web      - 启动前端开发服务 (localhost:5173, /api 代理到 :8080)"
	@echo "make dev-server   - 启动后端 API 服务 (localhost:8080)"
	@echo "make build-server - 编译后端 (当前平台)"
	@echo "make build-web    - 构建前端产物 (web/dist)"
	@echo "make build        - 构建前后端"
	@echo "make release      - 打包 linux/amd64 发布包 (release/*.tar.gz)"
	@echo "make clean        - 清理构建产物"

dev-server:
	cd server && go run . -addr :8080 -data ./data

dev-web:
	cd web && npm run dev

build-server:
	cd server && go build -ldflags="-s -w" -o parrot-intel-server .

build-web:
	cd web && npm run build

build: build-server build-web

release:
	VER=$(VER) bash scripts/release.sh

clean:
	rm -rf web/dist server/parrot-intel-server release
