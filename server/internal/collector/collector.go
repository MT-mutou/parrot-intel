// Package collector 实现 RSS 并发收集与 AI 分析调度。
package collector

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmcdole/gofeed"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/db"
)

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

func (c *Collector) run() {
	defer c.running.Store(false)
	start := time.Now()

	// ① 并发抓取 RSS
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

// fetchSource 抓取单个 RSS 源，返回新增文章数
func (c *Collector) fetchSource(s db.Source) (int, error) {
	fp := gofeed.NewParser()
	fp.UserAgent = "ParrotIntel/1.0 (+security-intel)"
	fp.Client = &http.Client{Timeout: 25 * time.Second}

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
