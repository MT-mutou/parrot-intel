# 🦜 鹦鹉情报站 ParrotIntel

威胁情报聚合平台 · **前后端分离架构** · Vue3 前端 + Go API 服务

从 **40+ 国内外安全信息源**（FreeBuf、安全客、The Hacker News、CISA 等）自动收集安全资讯，可选接入 AI（OpenAI 兼容：**DeepSeek / 通义 / Kimi / OpenAI**）自动生成中文摘要、威胁评级与通俗解读，每日简报**一键推送钉钉 / 企业微信 / 飞书 / 微信（Server酱）**。

## 架构（标准前后端分离）

```
浏览器
  │
  ▼
┌────────────────────────────────────────────┐
│ Nginx（80 端口）                            │
│  ├─ /        → 静态托管 web/dist（前端产物）│
│  └─ /api/    → 反向代理 → 127.0.0.1:8080    │
└──────────────────┬─────────────────────────┘
                   │
     ┌─────────────┴─────────────┐
     ▼                           │
┌──────────────────┐    ┌────────┴──────────┐
│ web/ 前端（独立） │    │ server/ 后端（独立）│
│ Vue3 + Element   │    │ Go RESTful API v1  │
│ Plus + ECharts   │    │ 收集引擎 · AI 管道  │
│ SPA，独立构建部署 │    │ 多渠道推送 · 定时   │
└──────────────────┘    └────────┬──────────┘
                                 │
                        SQLite（WAL，单文件）
                        /opt/parrot-intel/data
```

| 层 | 目录 | 技术 | 部署形态 |
|----|------|------|---------|
| 前端 | `web/` | Vue 3 · Vue Router · Pinia · Element Plus · ECharts · marked | Nginx 静态托管 `/var/www/parrot-intel` |
| 后端 | `server/` | Go 标准库路由 · modernc.org/sqlite（纯 Go）· gofeed | systemd 服务 `127.0.0.1:8080` |
| 数据 | — | SQLite（WAL）单文件 | `/opt/parrot-intel/data/cybersec.db` |
| 部署 | `deploy/` | Nginx 配置 · systemd 单元 · 安装/备份脚本 | `deploy.sh` 一键安装升级 |

### 仓库结构

```
parrot-intel/
├── server/                  # Go 后端（API 服务）
│   ├── main.go              # 入口：启动参数 / 定时收集 / 自动简报
│   └── internal/
│       ├── web/             # RESTful API v1（路由 + handlers + CORS + gzip）
│       ├── collector/       # RSS 并发收集引擎（去重 / 健康状态）
│       ├── ai/              # OpenAI 兼容客户端（摘要 / 评级 / 简报）
│       ├── notify/          # 多渠道推送（钉钉/企微/飞书/Server酱）
│       └── db/              # SQLite 数据层（40+ 预置源）
├── web/                     # Vue3 前端（独立工程）
│   ├── src/views/           # 态势总览/情报流/详情/简报/来源/渠道/设置
│   ├── src/api.js           # API 封装（VITE_API_BASE 可配置）
│   └── .env.example         # 环境变量示例
├── deploy/                  # 部署配置
│   ├── nginx.conf           # Nginx 站点（静态托管 + /api 反代 + gzip + SPA 回退）
│   ├── parrot-intel.service # systemd 单元（守护 + 内存保护 + 开机自启）
│   ├── deploy.sh            # 服务器端一键安装/升级/卸载
│   └── backup-db.sh         # SQLite 一致性备份（crontab 定时）
├── scripts/release.sh       # 开发机一键打包发布（前端构建+后端交叉编译）
└── Makefile                 # 构建/开发/发布编排
```

## 功能

- **情报收集**：40+ 国内外安全源（8 大分类）并发抓取，自动去重，来源健康状态监控，每日定时收集（时间可配）
- **AI 分析**（可选，OpenAI 兼容：DeepSeek / 通义 / Kimi / OpenAI）：每篇情报自动生成中文摘要、威胁评级（高/中/低）、分类标签、CVE 提取、💬「说人话」通俗解读；配置保存即时生效
- **每日简报**：AI 汇总当日情报生成管理层简报，历史简报按日回看
- **一键推送**：钉钉 / 企业微信 / 飞书 / 微信（Server酱）四渠道，卡片式管理、逐渠道测试、单渠道或全渠道推送；收集完成后可自动推送
- **态势总览**：关键指标卡、近 7 日趋势、威胁等级分布、分类分布、来源 TOP（ECharts）
- **情报流**：多维筛选（分类/威胁等级/未读/收藏）+ 搜索 + 分页，CVE 高亮，收藏/已读
- **来源管理**：增删启停、失败原因展示、40 源预置

## 本地开发

```bash
# 后端（:8080）
cd server && go run . -addr :8080 -data ./data

# 前端（:5173，/api 自动代理到 8080）
cd web && npm install && npm run dev

# 发布前预览（构建产物 + 反代模拟，等价于 Nginx 形态）
cd web && npm run build && API_PROXY_TARGET=http://127.0.0.1:8080 npm run preview
```

## 打包发布 → 部署（阿里云）

### ① 开发机打包（产出单个 tar.gz）

```bash
bash scripts/release.sh        # 或 make release
# → release/parrot-intel-v2.1.0.tar.gz（前端产物 + linux/amd64 后端 + 部署配置）
```

### ② 上传并一键部署（服务器需已安装 Nginx）

```bash
scp release/parrot-intel-v2.1.0.tar.gz root@服务器IP:/tmp/

# 服务器上：
mkdir -p /opt/parrot-intel-release
tar -xzf /tmp/parrot-intel-v2.1.0.tar.gz -C /opt/parrot-intel-release --strip-components=1
cd /opt/parrot-intel-release
sudo bash deploy/deploy.sh
```

`deploy.sh` 自动完成：安装后端到 `/opt/parrot-intel` → 发布前端到 `/var/www/parrot-intel` → 安装并启动 systemd 服务（守护 + 开机自启 + 健康检查）→ 安装 Nginx 站点配置（静态托管 + `/api` 反代 + gzip）。

完成后访问 `http://服务器IP/`（80 端口）。升级 = 重复上述三步（数据目录不受影响）。

### ③ 数据备份（建议）

```bash
# 安装备份脚本 + crontab（每天 03:00，保留 14 天）
sudo cp deploy/backup-db.sh /opt/parrot-intel/
(crontab -l 2>/dev/null; echo "0 3 * * * /opt/parrot-intel/backup-db.sh >> /opt/parrot-intel/logs/backup.log 2>&1") | crontab -
```

> `/opt/parrot-intel/data/cybersec.db` 是全部数据（含 AI Key、推送密钥），备份目录已限权 700。

## 运维速查

```bash
systemctl status parrot-intel           # 后端状态
journalctl -u parrot-intel -f           # 后端日志
systemctl restart parrot-intel          # 重启后端
curl http://127.0.0.1:8080/api/v1/health  # 健康检查
nginx -t && systemctl reload nginx      # 改 Nginx 配置后
sudo bash deploy/deploy.sh --uninstall  # 卸载（保留数据）
```

## 技术要点

- **纯 Go 无 CGO**：modernc.org/sqlite，服务器无需任何编译环境，交叉编译产物直接运行
- **前后端同域反代**：Nginx 同站点下 `/api` 反代，前端零配置；分开部署时后端已启用 CORS（`-cors` 参数），前端配 `VITE_API_BASE` 即可
- **性能**：API 全端点毫秒级（实测 1.6~3ms），25 秒并发收集 285 篇，gzip + 静态资源 immutable 长缓存
- **单实例运行**：定时任务在进程内调度（默认每日 09:00 收集），systemd 保证存活且勿起多实例
