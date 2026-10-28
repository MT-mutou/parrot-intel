#!/bin/bash
# 鹦鹉情报站 ParrotIntel —— 本地打包发布脚本（开发机执行）
# 产出 release/parrot-intel-v<版本>.tar.gz，上传服务器后解压执行 deploy/deploy.sh 即完成部署。
#
# 用法：bash scripts/release.sh          （打包 linux/amd64）
#       VER=2.1.1 bash scripts/release.sh
set -euo pipefail
cd "$(dirname "$0")/.."

VER="${VER:-2.2.2}"
OUT="release/parrot-intel-v${VER}"

echo ">> [1/4] 构建前端..."
cd web && npm run build >/dev/null && cd ..
[ -f web/dist/index.html ] || { echo "前端构建产物缺失"; exit 1; }

echo ">> [2/4] 交叉编译后端 (linux/amd64)..."
cd server
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "../${OUT}/server/parrot-intel-server" .
cd ..

echo ">> [3/4] 组装 release 包..."
rm -rf "${OUT}/web-dist"
mkdir -p "${OUT}/server" "${OUT}/web-dist" "${OUT}/deploy"
cp -r web/dist/* "${OUT}/web-dist/"
cp deploy/nginx.conf deploy/parrot-intel.service deploy/deploy.sh deploy/backup-db.sh "${OUT}/deploy/"

echo ">> [4/4] 压缩..."
mkdir -p release
tar -czf "release/parrot-intel-v${VER}.tar.gz" -C release "parrot-intel-v${VER}"
rm -rf "${OUT}"

SIZE=$(du -h "release/parrot-intel-v${VER}.tar.gz" | cut -f1)
echo
echo "完成 ✓  release/parrot-intel-v${VER}.tar.gz ($SIZE)"
echo
echo "部署步骤："
echo "  1. scp release/parrot-intel-v${VER}.tar.gz root@服务器IP:/tmp/"
echo "  2. 服务器: mkdir -p /opt/parrot-intel-release && tar -xzf /tmp/parrot-intel-v${VER}.tar.gz -C /opt/parrot-intel-release --strip-components=1"
echo "  3. 服务器: cd /opt/parrot-intel-release && sudo bash deploy/deploy.sh"
