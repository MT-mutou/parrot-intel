#!/bin/bash
# 鹦鹉情报站 ParrotIntel —— SQLite 一致性备份脚本
# crontab 示例（每天 03:00 备份，保留 14 天，避开默认 09:00 收集任务）：
#   0 3 * * * /opt/parrot-intel/backup-db.sh >> /opt/parrot-intel/logs/backup.log 2>&1
#
# 注意：不要直接 cp 数据库文件（写入中途复制可能损坏），必须用 .backup 一致性导出。
set -euo pipefail

DATA_DIR="/opt/parrot-intel/data"
BACKUP_DIR="/opt/parrot-intel/data/backups"
KEEP_DAYS=14

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"   # 备份含 AI Key / 推送密钥，限制访问

STAMP=$(date +%F)
sqlite3 "$DATA_DIR/cybersec.db" ".backup '$BACKUP_DIR/parrot-$(date +%F).db'"
find "$BACKUP_DIR" -name "parrot-*.db" -mtime +${KEEP_DAYS} -delete

echo "[$(date '+%F %T')] 备份完成：$BACKUP_DIR/parrot-$STAMP.db"
# 提醒：建议每周拷贝一份到服务器之外（异机容灾）
