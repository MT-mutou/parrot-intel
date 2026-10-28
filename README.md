# 🦜 鹦鹉情报站 ParrotIntel

威胁情报聚合平台 · **Go + Vue3 单二进制** · 毫秒级响应

从 **40+ 国内外安全信息源**（FreeBuf、安全客、The Hacker News、CISA 等）自动收集安全资讯，可选接入 AI（OpenAI 兼容：**DeepSeek / 通义 / Kimi / OpenAI**）自动生成中文摘要、威胁评级与通俗解读，每日简报**一键推送钉钉 / 企业微信 / 飞书 / 微信（Server酱）**。

## 架构

```
┌─────────────────────────────────────────────────┐
│                单二进制 parrot-intel              │
│  ┌───────────────────────────────────────────┐  │
│  │ Vue3 + Element Plus + ECharts（go:embed） │  │
│  │ 态势总览 / 情报流 / 简报 / 渠道 / 设置     │  │
│  └──────────────┬────────────────────────────┘  │
│                 │ RESTful /api/v1                 │
│  ┌──────────────┴────────────────────────────┐  │
│  │ Go 标准库路由 + gzip + 静态资源长缓存      │  │
│  │ 收集引擎（并发 RSS）· AI 分析管道          │  │
│  │ 多渠道推送（钉钉/企微/飞书/Server酱）      │  │
│  │ 定时任务（进程内，无需 crontab）           │  │
│  └──────────────┬────────────────────────────┘  │
│             SQLite（WAL，单文件）                 │
└─────────────────────────────────────────────────┘
```

| 层 | 技术 |
|----|------|
| 前端 | Vue 3 · Vue Router · Pinia · Element Plus · ECharts · marked |
| 后端 | Go 标准库路由 · modernc.org/sqlite（纯 Go，无 CGO）· gofeed |
| 性能 | SPA 静态资源 embed + immutable 缓存；API 3ms 内响应 |

## 性能指标

| 指标 | 表现 |
|------|------|
| 页面/路由响应 | **1.2~3ms** |
| API 响应 | **2~3ms** |
| 部署产物 | **单个 15MB 二进制**（前端全部内嵌） |
| 内存占用 | **~35MB** |
| 外部依赖 | **零**（无 CDN / 无字体下载 / 无 node_modules） |
| 收集速度 | 并发抓取 40 源 **25 秒 285 篇**（实测） |

## 快速开始

```bash
# 服务器上（amd64 Linux）
./parrot-intel -addr :8080 -data ./data
```

打开 `http://服务器IP:8080` → 点右上角「立即收集」。

## 功能

| 模块 | 说明 |
|------|------|
| **态势总览** | 近 7 日趋势 / 威胁等级分布 / 分类分布 / 来源 TOP10（ECharts） |
| **情报流** | 全文搜索、分类/威胁等级/未读/收藏多维筛选、CVE 高亮 |
| **AI 分析** | 每篇自动生成：中文摘要 + 分类 + 威胁评级（高/中/低）+ 标签 + 通俗解读 + CVE 提取 |
| **每日简报** | AI 汇总当日情报生成管理层简报，支持按日期查看历史 |
| **一键推送** | 钉钉 / 企业微信 / 飞书 / 微信（Server酱），支持逐渠道测试与单独推送 |
| **来源管理** | 40+ 预置源按类分组，支持增删改、启停、健康状态 |
| **定时任务** | 每日定时收集（小时可配）+ 可选自动生成简报并推送 |
| **数据备份** | 所有数据在 `data/parrot.db` 单文件（SQLite WAL） |

## 配置

1. **AI**：系统设置 → 填 OpenAI 兼容端点 + Key（DeepSeek / 通义 / Kimi 均可）。不配置则仅收集不分析，功能不受影响。
2. **推送**：推送渠道 → 添加机器人（钉钉/企微/飞书 webhook 或 Server酱 SendKey）→「测试推送」验证。
3. **定时**：系统设置 → 每日收集时间 + 自动推送简报开关。改完即时生效，无需重启。

## 部署（systemd 守护）

```bash
scp parrot-intel-linux-amd64 root@服务器:/opt/parrot-intel/parrot-intel

cat > /etc/systemd/system/parrot-intel.service << 'EOF'
[Unit]
Description=ParrotIntel
After=network.target

[Service]
WorkingDirectory=/opt/parrot-intel
ExecStart=/opt/parrot-intel/parrot-intel -addr :8080 -data /opt/parrot-intel/data
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload && systemctl enable --now parrot-intel
# 云安全组放行 TCP 8080
```

更新版本：替换二进制 → `systemctl restart parrot-intel`。

## 开发

```bash
# 前端（Vite 热更新，代理到 Go :8080）
cd webapp && npm install && npm run dev

# 后端
go run . -addr :8080 -data ./data

# 发布：先构建前端（产物输出到 internal/web/dist），再编译
cd webapp && npm run build && cd ..
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o parrot-intel-linux-amd64 .
```

## RESTful API（/api/v1）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /overview | 收集状态 + 关键指标 |
| GET | /articles | 情报列表（page/limit/category/threat/search/filter） |
| GET | /articles/{id} | 情报详情（自动标记已读） |
| POST | /articles/{id}/star · /read · /analyze | 收藏 / 已读 / AI 分析 |
| GET | /stats | 仪表盘统计 |
| GET/POST | /sources · /sources/{id}/toggle · DELETE | 来源管理 |
| GET/POST | /settings | 设置（Key 脱敏） |
| POST | /collect · GET /collect/status | 触发收集 / 状态轮询 |
| GET | /briefing?date= · /briefings | 简报内容 / 历史列表 |
| POST | /briefing/generate · /briefing/push | 生成 / 多渠道推送 |
| CRUD | /channels · /channels/{id}/test | 推送渠道管理与测试 |

## License

MIT
