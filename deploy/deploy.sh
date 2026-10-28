#!/bin/bash
# 鹦鹉情报站 ParrotIntel —— 服务器端安装/升级脚本
# 在 release 包解压后的目录中执行（包结构：server/ web-dist/ deploy/）
#
# 首次安装：  sudo bash deploy/deploy.sh
# 升级（保留数据）：sudo bash deploy/deploy.sh
# 卸载：      sudo bash deploy/deploy.sh --uninstall
set -euo pipefail

APP_NAME="parrot-intel"
INSTALL_DIR="/opt/parrot-intel"          # 后端 + 数据
WEB_DIR="/var/www/parrot-intel"          # 前端静态资源
SERVICE_SRC="deploy/parrot-intel.service"
SERVICE_DST="/etc/systemd/system/parrot-intel.service"
NGINX_SRC="deploy/nginx.conf"
NGINX_DST="/etc/nginx/conf.d/parrot-intel.conf"

[ "$(id -u)" -eq 0 ] || { echo "请用 sudo 运行"; exit 1; }
[ -f "server/parrot-intel-server" ] || { echo "缺少 server/parrot-intel-server（确认在 release 包解压目录内执行）"; exit 1; }
[ -d "web-dist" ] || { echo "缺少 web-dist/ 前端产物"; exit 1; }

# ---------- 卸载 ----------
if [ "${1:-}" = "--uninstall" ]; then
    echo ">> 停止并移除服务..."
    systemctl disable --now "$APP_NAME" 2>/dev/null || true
    rm -f "$SERVICE_DST"
    [ -f "$NGINX_DST" ] && mv "$NGINX_DST" "$NGINX_DST.bak.$(date +%s)"
    nginx -t 2>/dev/null && systemctl reload nginx || true
    echo ">> 已卸载服务与 Nginx 配置。"
    echo ">> 保留内容（含数据库，如需彻底删除请手动执行）："
    echo "   rm -rf $INSTALL_DIR $WEB_DIR"
    exit 0
fi

echo ">> [1/6] 安装后端到 $INSTALL_DIR ..."
mkdir -p "$INSTALL_DIR/data"
install -m 0755 server/parrot-intel-server "$INSTALL_DIR/parrot-intel-server"

echo ">> [2/6] 发布前端到 $WEB_DIR ..."
mkdir -p "$WEB_DIR"
rm -rf "$WEB_DIR"/*
cp -r web-dist/* "$WEB_DIR"/

echo ">> [3/6] 安装 systemd 服务 ..."
install -m 0644 "$SERVICE_SRC" "$SERVICE_DST"
systemctl daemon-reload
systemctl enable "$APP_NAME" >/dev/null

echo ">> [4/6] 启动/重启后端服务 ..."
systemctl restart "$APP_NAME"
sleep 1
if ! curl -sf http://127.0.0.1:8080/api/v1/health >/dev/null; then
    echo "!! 后端健康检查失败，查看日志：journalctl -u $APP_NAME -n 50"
    exit 1
fi
echo "   后端健康检查通过"

echo ">> [5/6] 配置 Nginx ..."
if ! command -v nginx >/dev/null; then
    echo "!! 未检测到 Nginx。请先安装：yum install -y nginx 或 apt install -y nginx"
    echo "   后端已在 127.0.0.1:8080 运行，可稍后手动配置。"
    exit 1
fi
install -m 0644 "$NGINX_SRC" "$NGINX_DST"
nginx -t
systemctl reload nginx

echo ">> [6/6] 完成 ✓"
echo
echo "   访问地址：http://$(hostname -I 2>/dev/null | awk '{print $1}')/  （Nginx 80 端口）"
echo "   后端状态：systemctl status $APP_NAME"
echo "   后端日志：journalctl -u $APP_NAME -f"
echo "   数据目录：$INSTALL_DIR/data（务必纳入备份）"
