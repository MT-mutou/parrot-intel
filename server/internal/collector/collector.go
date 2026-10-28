// Package collector 实现 RSS / 网页爬虫 / 开放 API 多类型并发收集与 AI 分析调度。
package collector

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/db"
)

// 浏览器 UA：部分安全站点（FreeBuf/Schneier 等）会拦截非浏览器标识的抓取
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 ParrotIntel/1.0"

// Collector 收集引擎
type Collector struct {
	DB  *db.DB
	AI  func() *ai.Client // 每次收集实时读取设置（改配置无需重启）
	Log *log.Logger

	// OnDone 收集完成后回调（可选；用于自动简报推送等）
	OnDone func(inserted, analyzed int)

	running atomic.Bool
	mu      sync.Mutex
	status  string
}

// Status 当前收集状态（前端轮询）
func (c *Collector) Status() (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running.Load(), c.status
}

func (c *Collector) setStatus(s string) {
	c.mu.Lock()
	c.status = s
	c.mu.Unlock()
}

// RunNow 触发一次收集（后台异步），ok=false 表示已在进行中
func (c *Collector) RunNow() bool {
	if !c.running.CompareAndSwap(false, true) {
		return false
	}
	go c.run()
	return true
}

// httpClient 每轮收集统一构建 HTTP 客户端；读取 collector_proxy 设置
// （境内服务器访问国际源（feedburner/blogspot 等）需配置代理）
func (c *Collector) httpClient() *http.Client {
	tr := &http.Transport{Proxy: nil}
	if pu := strings.TrimSpace(c.DB.GetSetting("collector_proxy", "")); pu != "" {
		if u, err := url.Parse(pu); err == nil && u.Scheme != "" {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: tr}
}

func (c *Collector) run() {
	defer c.running.Store(false)
	start := time.Now()

	// ① 并发抓取全部启用来源（RSS / 爬虫 / API）
	sources, err := c.DB.EnabledSources()
	if err != nil {
		c.setStatus("读取来源失败: " + err.Error())
		return
	}
	c.setStatus(fmt.Sprintf("抓取 %d 个来源…", len(sources)))

	var (
		inserted atomic.Int64
		wg       sync.WaitGroup
		sem      = make(chan struct{}, 10) // 并发上限
		failed   atomic.Int64
	)

	for _, src := range sources {
		wg.Add(1)
		go func(s db.Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			n, err := c.fetchSource(s)
			if err != nil {
				failed.Add(1)
				c.DB.UpdateSourceHealth(s.ID, "error", err.Error())
				c.Log.Printf("[collector] %s: %v", s.Name, err)
			} else {
				c.DB.UpdateSourceHealth(s.ID, "success", "")
			}
			inserted.Add(int64(n))
		}(src)
	}
	wg.Wait()

	// ② AI 分析未处理文章（已配置才执行）
	analyzed := 0
	if c.AI != nil {
		client := c.AI()
		if client != nil && client.Configured() {
			pending, _ := c.DB.Unanalyzed(100)
			for i, a := range pending {
				c.setStatus(fmt.Sprintf("AI 分析中 %d/%d…", i+1, len(pending)))
				input := a.Content
				if input == "" {
					input = a.Title
				}
				an, err := client.AnalyzeArticle(a.Title, input)
				if err != nil {
					c.Log.Printf("[ai] %s: %v", a.ID, err)
					continue
				}
				cves := extractCVEs(an.Summary + " " + strings.Join(an.Tags, " "))
				if err := c.DB.UpdateAnalysis(a.ID, an.Summary, an.Category, an.Region, an.ThreatLevel, an.Tags, cves, an.PlainExplanation); err != nil {
					c.Log.Printf("[ai] 写回失败 %s: %v", a.ID, err)
				}
				analyzed++
			}
		}
	}

	c.setStatus(fmt.Sprintf("完成：新增 %d 篇 · AI 分析 %d 篇 · 失败来源 %d 个 · 耗时 %s",
		inserted.Load(), analyzed, failed.Load(), time.Since(start).Round(time.Second)))
	c.Log.Printf("[collector] %s", c.status)

	if c.OnDone != nil {
		c.OnDone(int(inserted.Load()), analyzed)
	}
}

// fetchSource 按来源类型分发：rss / html（通用爬虫）/ nvd / ghsa
func (c *Collector) fetchSource(s db.Source) (int, error) {
	client := c.httpClient()
	switch s.Type {
	case "nvd":
		return c.fetchNVD(s, client)
	case "ghsa":
		return c.fetchGHSA(s, client)
	case "html":
		return c.fetchHTML(s, client)
	default:
		return c.fetchRSS(s, client)
	}
}

// ---------- RSS ----------

func (c *Collector) fetchRSS(s db.Source, client *http.Client) (int, error) {
	fp := gofeed.NewParser()
	fp.UserAgent = browserUA
	fp.Client = client

	feed, err := fp.ParseURL(s.URL)
	if err != nil {
		return 0, err
	}

	n := 0
	for _, item := range feed.Items {
		title := strings.TrimSpace(item.Title)
		link := item.Link
		if title == "" || link == "" {
			continue
		}
		pub := time.Now()
		if item.PublishedParsed != nil {
			pub = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			pub = *item.UpdatedParsed
		}

		summary := ""
		if item.Description != "" {
			summary = stripHTML(item.Description)
		}
		content := ""
		if item.Content != "" {
			content = stripHTML(item.Content)
		}

		id := hashID(link)
		if c.DB.InsertArticle(id, title, link, s.Name, s.URL, truncate(summary, 400), content, pub.Format(time.RFC3339)) {
			n++
		}
	}
	return n, nil
}

// ---------- NVD CVE API 2.0（官方开放接口，无需 Key）----------
// CNVD/CNNVD 官网无 RSS 且有反爬（WAF/登录），采用同为国家级漏洞库、
// 数据互相映射的 NVD 官方 API 获取最新 CVE。

func (c *Collector) fetchNVD(s db.Source, client *http.Client) (int, error) {
	api := s.URL
	if api == "" {
		api = "https://services.nvd.nist.gov/rest/json/cves/2.0"
	}
	// 最近 72 小时新发布的 CVE
	now := time.Now().UTC()
	start := now.Add(-72 * time.Hour)
	u := api + "?pubStartDate=" + url.QueryEscape(start.Format("2006-01-02T15:04:05.000")) +
		"&pubEndDate=" + url.QueryEscape(now.Format("2006-01-02T15:04:05.000")) +
		"&resultsPerPage=200"

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", browserUA)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("NVD API HTTP %d", resp.StatusCode)
	}

	var res struct {
		Vulnerabilities []struct {
			CVE struct {
				ID           string `json:"id"`
				Published    string `json:"published"`
				Descriptions []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"descriptions"`
				Metrics struct {
					CvssMetricV31 []struct {
						CvssData struct {
							BaseSeverity string `json:"baseSeverity"`
						} `json:"cvssData"`
					} `json:"cvssMetricV31"`
					CvssMetricV30 []struct {
						CvssData struct {
							BaseSeverity string `json:"baseSeverity"`
						} `json:"cvssData"`
					} `json:"cvssMetricV30"`
					CvssMetricV2 []struct {
						BaseSeverity string `json:"baseSeverity"`
					} `json:"cvssMetricV2"`
				} `json:"metrics"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}

	n := 0
	for _, v := range res.Vulnerabilities {
		cve := v.CVE
		desc := ""
		for _, d := range cve.Descriptions {
			if d.Lang == "en" {
				desc = d.Value
				break
			}
		}
		if desc == "" && len(cve.Descriptions) > 0 {
			desc = cve.Descriptions[0].Value
		}
		title := truncate(cve.ID+": "+firstSentence(desc), 200)
		link := "https://nvd.nist.gov/vuln/detail/" + cve.ID
		pub := time.Now()
		if t, err := time.Parse(time.RFC3339, cve.Published); err == nil {
			pub = t
		}

		sev := ""
		if len(cve.Metrics.CvssMetricV31) > 0 {
			sev = cve.Metrics.CvssMetricV31[0].CvssData.BaseSeverity
		} else if len(cve.Metrics.CvssMetricV30) > 0 {
			sev = cve.Metrics.CvssMetricV30[0].CvssData.BaseSeverity
		} else if len(cve.Metrics.CvssMetricV2) > 0 {
			sev = cve.Metrics.CvssMetricV2[0].BaseSeverity
		}

		id := hashID(link)
		if c.DB.InsertArticle(id, title, link, s.Name, s.URL, truncate(desc, 400), desc, pub.Format(time.RFC3339)) {
			// 结构化数据直接写威胁等级与 CVE 编号（不挡 AI 二次分析）
			c.DB.SetArticleMeta(id, severityToThreat(sev), []string{cve.ID}, []string{"NVD", "漏洞通报"})
			n++
		}
	}
	return n, nil
}

// ---------- GitHub Security Advisories（开源生态漏洞/供应链风险）----------

func (c *Collector) fetchGHSA(s db.Source, client *http.Client) (int, error) {
	api := s.URL
	if api == "" {
		api = "https://api.github.com/advisories"
	}
	u := api + "?per_page=50&sort=published&direction=desc"

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("GitHub Advisories API HTTP %d", resp.StatusCode)
	}

	var items []struct {
		GhsaID      string `json:"ghsa_id"`
		CveID       string `json:"cve_id"`
		HTMLURL     string `json:"html_url"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	n := 0
	for _, it := range items {
		pub := time.Now()
		if t, err := time.Parse(time.RFC3339, it.PublishedAt); err == nil {
			if t.Before(cutoff) { // 只收最近 7 天
				continue
			}
			pub = t
		}
		title := strings.TrimSpace(it.Summary)
		if title == "" {
			title = it.GhsaID
		}
		if it.CveID != "" {
			title = it.CveID + ": " + title
		}
		link := it.HTMLURL
		if link == "" {
			link = "https://github.com/advisories/" + it.GhsaID
		}
		content := stripMarkdown(it.Description)
		var cves []string
		if it.CveID != "" {
			cves = []string{it.CveID}
		}
		id := hashID(link)
		if c.DB.InsertArticle(id, truncate(title, 300), link, s.Name, s.URL, truncate(content, 400), content, pub.Format(time.RFC3339)) {
			c.DB.SetArticleMeta(id, severityToThreat(it.Severity), cves, []string{"GitHub", "供应链安全"})
			n++
		}
	}
	return n, nil
}

// ---------- 通用网页爬虫（CSS 选择器配置）----------

// htmlConfig 爬虫源配置（sources.config 列，JSON）
type htmlConfig struct {
	ListURL    string `json:"list_url"`    // 列表页地址（为空则用来源 URL）
	Item       string `json:"item"`        // 单条条目选择器（必填）
	Title      string `json:"title"`       // 标题选择器（条目内，必填）
	Link       string `json:"link"`        // 链接选择器（条目内，必填）
	LinkAttr   string `json:"link_attr"`   // 链接属性名，默认 href
	Date       string `json:"date"`        // 日期选择器（可选）
	DateFormat string `json:"date_format"` // 日期格式（Go 布局，可选）
	Summary    string `json:"summary"`     // 摘要选择器（可选）
	Base       string `json:"base"`        // 相对链接基准地址（可选）
}

func (c *Collector) fetchHTML(s db.Source, client *http.Client) (int, error) {
	cfg := htmlConfig{}
	if s.Config != nil && strings.TrimSpace(*s.Config) != "" {
		if err := json.Unmarshal([]byte(*s.Config), &cfg); err != nil {
			return 0, fmt.Errorf("爬虫配置 JSON 解析失败: %v", err)
		}
	}
	pageURL := cfg.ListURL
	if pageURL == "" {
		pageURL = s.URL
	}
	if pageURL == "" {
		return 0, errors.New("缺少 list_url（或来源 URL）")
	}
	if cfg.Item == "" || cfg.Title == "" || cfg.Link == "" {
		return 0, errors.New("爬虫配置缺少 item/title/link 选择器")
	}
	if cfg.LinkAttr == "" {
		cfg.LinkAttr = "href"
	}

	req, _ := http.NewRequest("GET", pageURL, nil)
	req.Header.Set("User-Agent", browserUA)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return 0, err
	}

	base := cfg.Base
	if base == "" {
		base = pageURL
	}
	baseURL, baseErr := url.Parse(base)

	n := 0
	doc.Find(cfg.Item).Each(func(_ int, sel *goquery.Selection) {
		title := strings.TrimSpace(sel.Find(cfg.Title).First().Text())
		link, _ := sel.Find(cfg.Link).First().Attr(cfg.LinkAttr)
		link = strings.TrimSpace(link)
		if title == "" || link == "" {
			return
		}
		// 相对链接 → 绝对链接
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") && baseErr == nil {
			if lu, err := url.Parse(link); err == nil {
				link = baseURL.ResolveReference(lu).String()
			}
		}
		summary := ""
		if cfg.Summary != "" {
			summary = strings.TrimSpace(sel.Find(cfg.Summary).First().Text())
		}
		pub := time.Now()
		if cfg.Date != "" {
			ds := strings.TrimSpace(sel.Find(cfg.Date).First().Text())
			if ds != "" {
				formats := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006/01/02 15:04", "2006-01-02", "2006/01/02", "2006.01.02", "2006年01月02日"}
				if cfg.DateFormat != "" {
					formats = append([]string{cfg.DateFormat}, formats...)
				}
				for _, f := range formats {
					if t, err := time.ParseInLocation(f, ds, time.Local); err == nil {
						pub = t
						break
					}
				}
			}
		}
		id := hashID(link)
		if c.DB.InsertArticle(id, truncate(title, 300), link, s.Name, s.URL, truncate(summary, 400), summary, pub.Format(time.RFC3339)) {
			n++
		}
	})
	if doc.Find(cfg.Item).Length() == 0 {
		return 0, fmt.Errorf("选择器 %q 未匹配到任何条目，请核对页面结构", cfg.Item)
	}
	return n, nil
}

// ---------- 工具函数 ----------

// severityToThreat CVSS/GHSA 严重级别 → 系统威胁等级
func severityToThreat(sev string) string {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL", "HIGH":
		return "high"
	case "MEDIUM", "MODERATE":
		return "medium"
	case "LOW":
		return "low"
	}
	return ""
}

// firstSentence 取第一句（标题用）
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{". ", "。", "\n"} {
		if i := strings.Index(s, sep); i > 20 {
			return s[:i+1]
		}
	}
	return s
}

// stripMarkdown 简易 Markdown 清理（GitHub 通告描述为 Markdown）
func stripMarkdown(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r", ""), "\n")
	var out []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		t = strings.TrimLeft(t, "#> ")
		t = strings.TrimPrefix(t, "- ")
		t = strings.TrimPrefix(t, "* ")
		if t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

// extractCVEs 提取 CVE 编号（去重）
var cveRegex = regexp.MustCompile(`CVE-\d{4}-\d{4,7}`)

func extractCVEs(text string) []string {
	m := cveRegex.FindAllString(strings.ToUpper(text), -1)
	seen := map[string]bool{}
	var out []string
	for _, c := range m {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func hashID(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:12])
}

var htmlTags = []string{"<p>", "</p>", "<br>", "<br/>", "<br />", "<div>", "</div>", "<a ", "</a>", "<strong>", "</strong>", "<em>", "</em>", "<li>", "</li>", "<ul>", "</ul>", "<h1>", "</h1>", "<h2>", "</h2>", "<h3>", "</h3>"}

func stripHTML(s string) string {
	s = strings.TrimSpace(s)
	for _, t := range htmlTags {
		s = strings.ReplaceAll(s, t, " ")
	}
	if idx := strings.Index(s, "<"); idx >= 0 {
		s = s[:idx] + " " + s[idx:]
	}
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
