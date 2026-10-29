package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// 模拟 360CERT 预警通告页（服务端渲染：标题链接 + 日期 + 摘要 + 「阅读全文」同链接）
const mock360HTML = `<!DOCTYPE html>
<html><body>
<div class="nav"><a href="/">首页</a> <a href="/warning">更多</a></div>
<ul class="warning-list">
  <li class="item">
    <a class="title" href="/warning/detail?id=661faf">Oracle 2026-04 补丁日多个产品漏洞安全风险通告</a>
    <span class="date">2026-04-17 19:18</span>
    <p class="desc">360CERT 监测发现 Oracle 发布了 2026 年 04 月份的风险通告，事件等级：严重。</p>
    <a href="/warning/detail?id=661faf">阅读全文</a>
  </li>
  <li class="item">
    <a class="title" href="/warning/detail?id=660a8f">安全事件周报 2026-03-25 第13周</a>
    <span class="date">2026-04-01 18:42</span>
    <p class="desc">本周收录安全热点 62 项。</p>
    <a href="/warning/detail?id=660a8f">阅读全文</a>
  </li>
  <li class="item">
    <a class="title" href="/news/other">一条不相关的新闻链接</a>
  </li>
</ul>
</body></html>`

// 模拟阿里云 AVD 高危漏洞库列表（表格行，链接文本仅 AVD 编号，需 container 模式取整行标题）
const mockAVDHTML = `<!DOCTYPE html>
<html><body>
<table class="table">
<tbody>
  <tr>
    <td><a href="/detail?id=AVD-2026-111497">AVD-2026-111497</a></td>
    <td>Node.js Brotli 解码器远程代码执行漏洞（CVE-2026-111497）</td>
    <td>CWE-252</td>
    <td>2026-10-22</td>
    <td>CVE PoC</td>
  </tr>
  <tr>
    <td><a href="/detail?id=AVD-2026-109618">AVD-2026-109618</a></td>
    <td>PHP session.upload_progress 远程代码执行漏洞（CVE-2026-109618）</td>
    <td>CWE-1325</td>
    <td>2026-10-21</td>
    <td>CVE PoC</td>
  </tr>
</tbody>
</table>
</body></html>`

// 模拟安恒星图资讯流（相对时间「N天前」，条目链接 /info/<id>）
const mockAnhengHTML = `<!DOCTYPE html>
<html><body>
<div class="feed">
  <div class="post">
    <a class="post-title" href="https://starmap.dbappsecurity.com.cn/info/16779">微软紧急发布.NET安全更新 修复Google Brotli压缩库高危漏洞</a>
    <div class="meta">安恒恒脑 <span>1天前</span> 0 119</div>
    <div class="tags">零日漏洞 系统漏洞利用</div>
  </div>
  <div class="post">
    <a class="post-title" href="https://starmap.dbappsecurity.com.cn/info/16778">LiteLLM供应链攻击溯源：暴露的主密钥引发大规模凭证窃取</a>
    <div class="meta">安恒恒脑 <span>3小时前</span> 0 114</div>
    <div class="tags">供应链攻击 数据泄露</div>
  </div>
</div>
</body></html>`

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("解析 HTML 失败: %v", err)
	}
	return doc
}

func TestParseSmartlinks360(t *testing.T) {
	doc := mustDoc(t, mock360HTML)
	cfg := smartlinkConfig{LinkPattern: "/warning/detail", MaxItems: 100}
	entries := parseSmartlinks(doc, cfg, "https://cert.360.cn/warning")
	if len(entries) != 2 {
		t.Fatalf("应提取 2 条（同 href 的「阅读全文」去重、不相关链接过滤），实际 %d 条", len(entries))
	}
	e := entries[0]
	if !strings.Contains(e.Title, "Oracle") {
		t.Fatalf("标题应取链接文本，实际: %q", e.Title)
	}
	if e.Link != "https://cert.360.cn/warning/detail?id=661faf" {
		t.Fatalf("相对链接应解析为绝对地址，实际: %q", e.Link)
	}
	want := time.Date(2026, 4, 17, 19, 18, 0, 0, time.Local)
	if !e.Pub.Equal(want) {
		t.Fatalf("日期应为 2026-04-17 19:18，实际 %v", e.Pub)
	}
	// 第二条日期 2026-04-01 18:42
	if !entries[1].Pub.Equal(time.Date(2026, 4, 1, 18, 42, 0, 0, time.Local)) {
		t.Fatalf("第二条日期应解析父容器文本，实际 %v", entries[1].Pub)
	}
}

func TestParseSmartlinksAVDContainer(t *testing.T) {
	doc := mustDoc(t, mockAVDHTML)
	cfg := smartlinkConfig{LinkPattern: "detail?id=AVD-", TitleMode: "container", MaxItems: 100}
	entries := parseSmartlinks(doc, cfg, "https://avd.aliyun.com/high-risk/list")
	if len(entries) != 2 {
		t.Fatalf("应提取 2 条，实际 %d", len(entries))
	}
	e := entries[0]
	if !strings.Contains(e.Title, "AVD-2026-111497") || !strings.Contains(e.Title, "Node.js Brotli") {
		t.Fatalf("container 模式标题应含编号与漏洞名称，实际: %q", e.Title)
	}
	// 日期应截掉：标题里不应出现 2026-10-22
	if strings.Contains(e.Title, "2026-10-22") {
		t.Fatalf("container 标题应截掉日期部分，实际: %q", e.Title)
	}
	if !e.Pub.Equal(time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("日期应为 2026-10-22，实际 %v", e.Pub)
	}
	if !strings.HasPrefix(e.Link, "https://avd.aliyun.com/detail?id=AVD-") {
		t.Fatalf("链接应为绝对地址，实际: %q", e.Link)
	}
}

func TestParseSmartlinksRelativeDate(t *testing.T) {
	doc := mustDoc(t, mockAnhengHTML)
	cfg := smartlinkConfig{LinkPattern: "/info/", MaxItems: 100}
	before := time.Now()
	entries := parseSmartlinks(doc, cfg, "https://starmap.dbappsecurity.com.cn/info")
	if len(entries) != 2 {
		t.Fatalf("应提取 2 条，实际 %d", len(entries))
	}
	// 「1天前」
	d1 := before.Sub(entries[0].Pub)
	if d1 < 23*time.Hour || d1 > 25*time.Hour {
		t.Fatalf("「1天前」应解析为约 24 小时前，实际偏差 %v", d1)
	}
	// 「3小时前」
	d2 := before.Sub(entries[1].Pub)
	if d2 < 2*time.Hour+50*time.Minute || d2 > 3*time.Hour+10*time.Minute {
		t.Fatalf("「3小时前」应解析为约 3 小时前，实际偏差 %v", d2)
	}
	if entries[0].Link != "https://starmap.dbappsecurity.com.cn/info/16779" {
		t.Fatalf("绝对链接保持不变，实际: %q", entries[0].Link)
	}
}

func TestParseSmartlinksNoMatch(t *testing.T) {
	doc := mustDoc(t, mock360HTML)
	cfg := smartlinkConfig{LinkPattern: "/not-exist", MaxItems: 100}
	if entries := parseSmartlinks(doc, cfg, "https://example.com"); len(entries) != 0 {
		t.Fatalf("不匹配的特征应返回 0 条，实际 %d", len(entries))
	}
}

func TestParseSmartlinksChineseDate(t *testing.T) {
	html := `<div><a href="/psirt/security-notices/2026/sn-1">关于PHP2Shell释放后使用漏洞的声明</a>
		<span>2026年10月28日</span></div>`
	doc := mustDoc(t, html)
	cfg := smartlinkConfig{LinkPattern: "/psirt/security-", MaxItems: 100}
	entries := parseSmartlinks(doc, cfg, "https://www.huawei.com/cn/psirt")
	if len(entries) != 1 {
		t.Fatalf("应提取 1 条，实际 %d", len(entries))
	}
	if !entries[0].Pub.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("中文日期应解析为 2026-10-28，实际 %v", entries[0].Pub)
	}
	if !strings.Contains(entries[0].Title, "PHP2Shell") {
		t.Fatalf("标题应取链接文本，实际: %q", entries[0].Title)
	}
}
