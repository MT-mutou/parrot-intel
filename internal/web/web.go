// Package web 提供 HTTP 服务：SSR 页面 + 轻量 JSON API。
package web

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
	"parrot-intel/internal/notify"
)

//go:embed templates/*.html
var tplFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server Web 服务
type Server struct {
	DB    *db.DB
	Col   *collector.Collector
	Log   *log.Logger
	pages map[string]*template.Template
}

// New 构造并预编译模板（每个页面一个独立模板集，共享 layout）
func New(d *db.DB, c *collector.Collector, lg *log.Logger) (*Server, error) {
	funcs := template.FuncMap{
		"threatClass": threatClass,
		"threatLabel": threatLabel,
		"fmtTime":     fmtTime,
		"add1":        func(n int) int { return n + 1 },
		"deref":       func(s *string) string { if s == nil { return "" }; return *s },
		"mul":         func(a, b, c int) int { if c == 0 { return 0 }; return a * b / c },
		"safeMD":      safeMD,
	}
	pages := map[string]*template.Template{}
	for _, name := range []string{"index", "article", "sources", "settings", "briefing", "dashboard"} {
		t, err := template.New(name).Funcs(funcs).ParseFS(tplFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("解析模板 %s: %w", name, err)
		}
		pages[name] = t
	}
	return &Server{DB: d, Col: c, Log: lg, pages: pages}, nil
}

// Handler 路由总入口（Go 1.22+ 方法路由）
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 静态资源（embed，带长缓存）
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("embed static 目录缺失: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// 页面
	mux.HandleFunc("GET /{$}", s.pageIndex)
	mux.HandleFunc("GET /article/{id}", s.pageArticle)
	mux.HandleFunc("GET /sources", s.pageSources)
	mux.HandleFunc("GET /settings", s.pageSettings)
	mux.HandleFunc("POST /settings", s.saveSettings)
	mux.HandleFunc("GET /briefing", s.pageBriefing)
	mux.HandleFunc("GET /dashboard", s.pageDashboard)

	// JSON API
	mux.HandleFunc("POST /api/star/{id}", s.apiStar)
	mux.HandleFunc("POST /api/read/{id}", s.apiRead)
	mux.HandleFunc("POST /api/collect", s.apiCollect)
	mux.HandleFunc("GET /api/collect/status", s.apiCollectStatus)
	mux.HandleFunc("POST /api/sources", s.apiSources)
	mux.HandleFunc("POST /api/analyze/{id}", s.apiAnalyze)
	mux.HandleFunc("POST /api/briefing/generate", s.apiBriefingGenerate)
	mux.HandleFunc("POST /api/briefing/push", s.apiBriefingPush)

	return gzipMiddleware(mux)
}

// ---------- 基础数据 ----------

type baseData struct {
	Title    string
	Active   string
	ColRunning bool
	ColStatus  string
}

func (s *Server) base(title, active string) baseData {
	running, status := s.Col.Status()
	return baseData{Title: title, Active: active, ColRunning: running, ColStatus: status}
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.pages[page]
	if !ok {
		s.Log.Printf("[web] 未知页面 %s", page)
		return
	}
	// 渲染共享 layout，其内部 {{template "content" .}} 由该页独立模板集提供
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.Log.Printf("[web] render %s: %v", page, err)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---------- 页面 handlers ----------

type indexData struct {
	baseData
	Articles   []db.Article
	Total      int
	Page       int
	TotalPages int
	Category   string
	Threat     string
	Search     string
	Filter     string // all / starred / unread
	Categories []string
}

func (s *Server) pageIndex(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	data := indexData{
		baseData:   s.base("情报流 · 鹦鹉情报站", "index"),
		Page:       page,
		Category:   q.Get("category"),
		Threat:     q.Get("threat"),
		Search:     q.Get("q"),
		Filter:     q.Get("filter"),
		Categories: []string{"安全事件·漏洞", "技术研究", "政策法规", "厂商动态", "AI安全"},
	}

	arts, total, err := s.DB.ListArticles(db.ListFilter{
		Page:     page,
		Limit:    25,
		Category: data.Category,
		Threat:   data.Threat,
		Search:   data.Search,
		Starred:  data.Filter == "starred",
		Unread:   data.Filter == "unread",
	})
	if err != nil {
		s.Log.Printf("[web] list: %v", err)
	}
	data.Articles = arts
	data.Total = total
	data.TotalPages = (total + 24) / 25
	if data.TotalPages < 1 {
		data.TotalPages = 1
	}
	s.render(w, "index", data)
}

func (s *Server) pageArticle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.DB.GetArticle(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.DB.MarkRead(id) // 打开即已读
	a.IsRead = 1
	s.render(w, "article", struct {
		baseData
		A *db.ArticleDetail
	}{s.base(a.Title+" · 鹦鹉情报站", "index"), a})
}

type sourcesData struct {
	baseData
	Sources []db.Source
}

func (s *Server) pageSources(w http.ResponseWriter, r *http.Request) {
	list, _ := s.DB.ListSources()
	s.render(w, "sources", sourcesData{s.base("来源管理 · 鹦鹉情报站", "sources"), list})
}

func (s *Server) pageSettings(w http.ResponseWriter, r *http.Request) {
	all := s.DB.AllSettings()
	data := struct {
		baseData
		Endpoint, Model, APIKeySet, DingTokenSet, DingSecretSet, ServerURL string
		CollectHour string
		Saved       bool
	}{
		baseData:    s.base("系统设置 · 鹦鹉情报站", "settings"),
		Endpoint:    all["ai_endpoint"],
		Model:       all["ai_model"],
		APIKeySet:   maskKey(all["ai_api_key"]),
		DingTokenSet: maskKey(all["ding_token"]),
		DingSecretSet: maskKey(all["ding_secret"]),
		ServerURL:   all["server_url"],
		CollectHour: atoiOr(all["collect_hour"], "9"),
		Saved:       r.URL.Query().Get("saved") == "1",
	}
	if data.Endpoint == "" {
		data.Endpoint = "https://api.deepseek.com/v1"
	}
	if data.Model == "" {
		data.Model = "deepseek-chat"
	}
	s.render(w, "settings", data)
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "表单错误", 400)
		return
	}
	set := func(key string) {
		if v := strings.TrimSpace(r.FormValue(key)); v != "" {
			s.DB.SetSetting(key, v)
		}
	}
	set("ai_endpoint")
	set("ai_model")
	set("ai_api_key") // 留空=不修改
	set("ding_token")
	set("ding_secret")
	set("server_url")
	set("collect_hour")
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *Server) pageBriefing(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	content, _ := s.DB.GetBriefing(date)
	s.render(w, "briefing", struct {
		baseData
		Date, Content string
	}{s.base("每日简报 · 鹦鹉情报站", "briefing"), date, content})
}

func (s *Server) pageDashboard(w http.ResponseWriter, r *http.Request) {
	st, _ := s.DB.GetStats()
	s.render(w, "dashboard", struct {
		baseData
		Stats *db.Stats
	}{s.base("态势总览 · 鹦鹉情报站", "dashboard"), st})
}

// ---------- API handlers ----------

func (s *Server) apiStar(w http.ResponseWriter, r *http.Request) {
	starred, err := s.DB.ToggleStar(r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"starred": starred})
}

func (s *Server) apiRead(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.MarkRead(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiCollect(w http.ResponseWriter, r *http.Request) {
	if !s.Col.RunNow() {
		writeJSON(w, map[string]any{"started": false, "msg": "收集已在进行中"})
		return
	}
	writeJSON(w, map[string]any{"started": true})
}

func (s *Server) apiCollectStatus(w http.ResponseWriter, r *http.Request) {
	running, status := s.Col.Status()
	writeJSON(w, map[string]any{"running": running, "status": status})
}

func (s *Server) apiSources(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action   string `json:"action"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		Category string `json:"category"`
		ID       int64  `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	switch req.Action {
	case "add":
		if req.Name == "" || req.URL == "" {
			writeErr(w, 400, "名称和 URL 必填")
			return
		}
		if err := s.DB.AddSource(req.Name, req.URL, req.Category); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	case "toggle":
		s.DB.ToggleSource(req.ID)
	case "delete":
		s.DB.DeleteSource(req.ID)
	default:
		writeErr(w, 400, "未知操作")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) aiClient() *ai.Client {
	all := s.DB.AllSettings()
	return ai.New(ai.Config{
		Endpoint: orDefault(all["ai_endpoint"], "https://api.deepseek.com/v1"),
		APIKey:   all["ai_api_key"],
		Model:    orDefault(all["ai_model"], "deepseek-chat"),
	})
}

func (s *Server) apiAnalyze(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.DB.GetArticle(id)
	if err != nil {
		writeErr(w, 404, "文章不存在")
		return
	}
	c := s.aiClient()
	if !c.Configured() {
		writeErr(w, 400, "未配置 AI API Key，请到系统设置")
		return
	}
	content := a.Content
	if content == "" {
		content = a.Title
	}
	an, err := c.AnalyzeArticle(a.Title, content)
	if err != nil {
		writeErr(w, 502, "AI 分析失败："+err.Error())
		return
	}
	// CVE 提取
	cves := extractCVEsLocal(an.Summary + " " + strings.Join(an.Tags, " "))
	s.DB.UpdateAnalysis(a.ID, an.Summary, an.Category, an.Region, an.ThreatLevel, an.Tags, cves, an.PlainExplanation)
	writeJSON(w, an)
}

func (s *Server) apiBriefingGenerate(w http.ResponseWriter, r *http.Request) {
	items, err := s.DB.TodayArticles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(items) == 0 {
		writeErr(w, 400, "今日暂无文章，请先收集")
		return
	}
	c := s.aiClient()
	if !c.Configured() {
		writeErr(w, 400, "未配置 AI API Key，请到系统设置")
		return
	}
	var bis []ai.BriefingItem
	for _, it := range items {
		sum := ""
		if it.Summary != nil {
			sum = *it.Summary
		}
		threat := ""
		if it.ThreatLevel != nil {
			threat = *it.ThreatLevel
		}
		bis = append(bis, ai.BriefingItem{Title: it.Title, Summary: sum, Threat: threat})
	}
	md, err := c.GenerateBriefing(bis)
	if err != nil {
		writeErr(w, 502, "简报生成失败："+err.Error())
		return
	}
	date := time.Now().Format("2006-01-02")
	s.DB.SaveBriefing(date, md)
	writeJSON(w, map[string]string{"date": date, "content": md})
}

func (s *Server) apiBriefingPush(w http.ResponseWriter, r *http.Request) {
	all := s.DB.AllSettings()
	dt := notify.New(all["ding_token"], all["ding_secret"])
	if !dt.Configured() {
		writeErr(w, 400, "未配置钉钉机器人 Token，请到系统设置")
		return
	}
	date := time.Now().Format("2006-01-02")
	content, _ := s.DB.GetBriefing(date)
	if content == "" {
		writeErr(w, 400, "今日简报尚未生成，请先生成")
		return
	}
	if err := dt.SendMarkdown("鹦鹉情报站 · 每日安全简报 "+date, notify.TruncateMD(notify.BriefingToMarkdown(content), 18000)); err != nil {
		writeErr(w, 502, "推送失败："+err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ---------- helpers ----------

var cveRegex = regexp.MustCompile(`CVE-\d{4}-\d{4,7}`)

func extractCVEsLocal(text string) []string {
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

func threatClass(level *string) string {
	if level == nil {
		return "t-none"
	}
	switch *level {
	case "high":
		return "t-high"
	case "medium":
		return "t-medium"
	case "low":
		return "t-low"
	}
	return "t-none"
}

func threatLabel(level *string) string {
	if level == nil {
		return ""
	}
	switch *level {
	case "high":
		return "高危"
	case "medium":
		return "中危"
	case "low":
		return "低危"
	}
	return ""
}

func fmtTime(s *string) string {
	if s == nil || *s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return *s
	}
	return t.Local().Format("01-02 15:04")
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:4] + "••••" + k[len(k)-4:]
}

func atoiOr(s, def string) string {
	if s == "" {
		return def
	}
	if _, err := strconv.Atoi(s); err != nil {
		return def
	}
	return s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func safeMD(s string) template.HTML {
	// 简单 markdown → HTML（简报展示：标题/加粗/列表/段落）
	var b strings.Builder
	lines := strings.Split(s, "\n")
	inList := false
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(ln, "# "):
			b.WriteString("<h2>" + esc(ln[2:]) + "</h2>")
		case strings.HasPrefix(ln, "## "):
			b.WriteString("<h3>" + esc(ln[3:]) + "</h3>")
		case strings.HasPrefix(ln, "### "):
			b.WriteString("<h4>" + esc(ln[4:]) + "</h4>")
		case strings.HasPrefix(ln, "- ") || strings.HasPrefix(ln, "* "):
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + esc(ln[2:]) + "</li>")
		case strings.HasPrefix(ln, "> "):
			b.WriteString("<blockquote>" + esc(ln[2:]) + "</blockquote>")
		case ln == "":
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
		default:
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			b.WriteString("<p>" + esc(ln) + "</p>")
		}
	}
	if inList {
		b.WriteString("</ul>")
	}
	return template.HTML(b.String())
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	// **bold**
	var out strings.Builder
	parts := strings.Split(s, "**")
	for i, p := range parts {
		if i%2 == 1 {
			out.WriteString("<strong>" + p + "</strong>")
		} else {
			out.WriteString(p)
		}
	}
	return out.String()
}

// ---------- gzip ----------

type gzipWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.URL.Path == "/api/collect" {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzipWriter{w, gz}, r)
	})
}
