# 🦜 鹦鹉情报站 ParrotIntel

威胁情报聚合平台 · **Go 单二进制** · 毫秒级响应

从 40+ 国内外安全信息源（FreeBuf、安全客、The Hacker News、CISA 等）自动收集安全资讯，可选接入 AI（OpenAI 兼容：DeepSeek / 通义 / Kimi）自动生成中文摘要、威胁评级与通俗解读，支持每日简报一键推送钉钉。

## 为什么快

| 指标 | 表现 |
|------|------|
| 首页响应 | **3-5ms**（服务端直出 HTML） |
| 部署产物 | **单个 15MB 二进制**（模板/样式/脚本全部内嵌） |
| 内存占用 | **~30MB** |
| 外部依赖 | **零**（无 CDN、无 Google Fonts、无 node_modules） |
| 收集速度 | 并发抓取 20 源 **6 秒 215 篇** |

## 快速开始

```bash
# 服务器上（amd64 Linux）
./parrot-intel -addr :8080 -data ./data
```

打开 `http://服务器IP:8080`，点右上角「立即收集」即可抓取首批情报。

## 部署（systemd 守护）

```bash
# 1. 上传二进制
scp parrot-intel-linux-amd64 root@服务器:/opt/parrot-intel/parrot-intel

# 2. 创建 systemd 服务
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

# 3. 启动 + 开机自启
systemctl daemon-reload
systemctl enable --now parrot-intel
systemctl status parrot-intel

# 4. 防火墙/云安全组放行 TCP 8080
```

更新版本：替换二进制 → `systemctl restart parrot-intel`。

## 功能配置

### AI 分析（设置页）
收集后自动为文章生成中文摘要 / 分类 / 威胁评级（高/中/低）/ 标签 / 通俗解读，并提取 CVE 编号。不配置则仅收集原文，功能不受影响。

### 每日简报（简报页）
AI 汇总当日全部情报 → 管理层友好的 markdown 简报 → 一键推送钉钉群（需在设置页配置机器人 Token + 加签 Secret）。

### 定时收集
设置页配置每日收集时间（默认 09:00），进程内自动调度，无需 crontab。

### 数据备份
所有数据在 `data/parrot.db` 单文件（SQLite WAL）。停服拷贝或：
```bash
sqlite3 data/parrot.db ".backup '/备份目录/parrot-$(date +%F).db'"
```

## 构建与开发

```bash
# 本地运行
go run . -addr :8080 -data ./data

# 交叉编译 Linux（CGO 免费，纯 Go SQLite）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o parrot-intel-linux-amd64 .
```

技术栈：Go 1.22+ 标准库路由 + html/template SSR + modernc.org/sqlite（纯 Go）+ gofeed。前端零依赖原生 CSS/JS（<5KB）。

## 与 cybersec-daily（上一代）对比

| 维度 | cybersec-daily (Next.js) | ParrotIntel (Go) |
|------|--------------------------|------------------|
| 首屏 | CSR 白屏后 fetch | SSR 毫秒级直出 |
| 部署 | node_modules + build | 单文件 scp |
| 内存 | 300-500MB | ~30MB |
| 依赖风险 | npm 生态 / Google Fonts | 零外部资源 |
| AI / 简报 / 钉钉 / 收藏 / 来源管理 | ✅ | ✅ 全部保留 |

## License

MIT
