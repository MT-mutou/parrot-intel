// RESTful API v1 handlers。
package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/db"
	"parrot-intel/internal/notify"
)

// fsSub dist 子文件系统
func fsSub() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// fsReadFile 从 embed dist 读文件
func fsReadFile(name string) ([]byte, error) {
	return distFS.ReadFile("dist/" + name)
}

// ---------- overview / stats ----------

// GET /api/v1/overview —— 顶栏状态 + 关键指标
func (s *Server) apiOverview(w http.ResponseWriter, r *http.Request) {
	running, status := s.Col.Status()
	st, _ := s.DB.GetStats()
	writeJSON(w, map[string]any{
		"running": running,
		"status":  status,
		"stats":   st,
	})
}

// GET /api/v1/stats —— 仪表盘统计
func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.DB.GetStats()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, st)
}

// ---------- articles ----------

// GET /api/v1/articles?page=&limit=&category=&threat=&search=&source=&filter=all|starred|unread
func (s *Server) apiListArticles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	filter := q.Get("filter")

	arts, total, err := s.DB.ListArticles(db.ListFilter{
		Page:     page,
		Limit:    limit,
		Category: q.Get("category"),
		Threat:   q.Get("threat"),
		Search:   q.Get("search"),
		Source:   q.Get("source"),
		Starred:  filter == "starred",
		Unread:   filter == "unread",
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if limit < 1 {
		limit = 25
	}
	writeJSON(w, map[string]any{
		"articles":   arts,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"total_pages": (total + limit - 1) / limit,
	})
}

// GET /api/v1/articles/{id} —— 详情（同时标记已读）
func (s *Server) apiGetArticle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.DB.GetArticle(id)
	if err != nil {
		writeErr(w, 404, "文章不存在")
		return
	}
	s.DB.MarkRead(id)
	a.IsRead = 1
	writeJSON(w, a)
}

// POST /api/v1/articles/{id}/star
func (s *Server) apiStar(w http.ResponseWriter, r *http.Request) {
	starred, err := s.DB.ToggleStar(r.PathValue("id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"starred": starred})
}

// POST /api/v1/articles/{id}/read
func (s *Server) apiRead(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.MarkRead(r.PathValue("id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/articles/{id}/analyze —— AI 深度分析单篇
func (s *Server) apiAnalyze(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.DB.GetArticle(id)
	if err != nil {
		writeErr(w, 404, "文章不存在")
		return
	}
	c := s.AI()
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
	cves := extractCVEs(an.Summary + " " + strings.Join(an.Tags, " "))
	s.DB.UpdateAnalysis(a.ID, an.Summary, an.Category, an.Region, an.ThreatLevel, an.Tags, cves, an.PlainExplanation)
	writeJSON(w, an)
}

// ---------- sources ----------

// GET /api/v1/sources
func (s *Server) apiListSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListSources()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, list)
}

// POST /api/v1/sources {name,url,category}
func (s *Server) apiAddSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Category string `json:"category"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		writeErr(w, 400, "名称和 URL 必填")
		return
	}
	if err := s.DB.AddSource(req.Name, req.URL, req.Category); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/sources/{id}/toggle
func (s *Server) apiToggleSource(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.DB.ToggleSource(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// DELETE /api/v1/sources/{id}
func (s *Server) apiDeleteSource(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.DB.DeleteSource(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ---------- settings ----------

// GET /api/v1/settings —— 脱敏返回
func (s *Server) apiGetSettings(w http.ResponseWriter, r *http.Request) {
	all := s.DB.AllSettings()
	get := func(k, def string) string {
		if v, ok := all[k]; ok && v != "" {
			return v
		}
		return def
	}
	writeJSON(w, map[string]any{
		"ai_endpoint":       get("ai_endpoint", "https://api.deepseek.com/v1"),
		"ai_model":          get("ai_model", "deepseek-chat"),
		"ai_api_key_set":    all["ai_api_key"] != "",
		"server_url":        all["server_url"],
		"collect_hour":      get("collect_hour", "9"),
		"auto_briefing_push": all["auto_briefing_push"] == "true",
	})
}

// POST /api/v1/settings —— 空字段=不修改（key 类）
func (s *Server) apiSaveSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	strKeys := []string{"ai_endpoint", "ai_model", "ai_api_key", "server_url", "collect_hour"}
	for _, k := range strKeys {
		if v, ok := req[k]; ok && strings.TrimSpace(v) != "" {
			s.DB.SetSetting(k, strings.TrimSpace(v))
		}
	}
	if v, ok := req["auto_briefing_push"]; ok {
		s.DB.SetSetting("auto_briefing_push", v)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// ---------- collect ----------

// POST /api/v1/collect
func (s *Server) apiCollect(w http.ResponseWriter, r *http.Request) {
	if !s.Col.RunNow() {
		writeJSON(w, map[string]any{"started": false, "msg": "收集已在进行中"})
		return
	}
	writeJSON(w, map[string]any{"started": true})
}

// GET /api/v1/collect/status
func (s *Server) apiCollectStatus(w http.ResponseWriter, r *http.Request) {
	running, status := s.Col.Status()
	writeJSON(w, map[string]any{"running": running, "status": status})
}

// ---------- briefing ----------

// GET /api/v1/briefing?date=2006-01-02
func (s *Server) apiGetBriefing(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	content, err := s.DB.GetBriefing(date)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]string{"date": date, "content": content})
}

// GET /api/v1/briefings —— 历史简报列表
func (s *Server) apiBriefingList(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListBriefingDates(30)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, list)
}

// POST /api/v1/briefing/generate {date?}
func (s *Server) apiBriefingGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date string `json:"date"`
	}
	_ = decodeBody(r, &req)
	date := req.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	items, err := s.DB.TodayArticles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(items) == 0 {
		writeErr(w, 400, "今日暂无文章，请先收集")
		return
	}
	c := s.AI()
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
	s.DB.SaveBriefing(date, md)
	writeJSON(w, map[string]string{"date": date, "content": md})
}

// POST /api/v1/briefing/push {date?, channel_ids?}
func (s *Server) apiBriefingPush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date       string  `json:"date"`
		ChannelIDs []int64 `json:"channel_ids"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	date := req.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	content, _ := s.DB.GetBriefing(date)
	if content == "" {
		writeErr(w, 400, "当日简报尚未生成，请先生成")
		return
	}

	channels, err := s.DB.ListChannels()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 过滤目标渠道
	targets := channels
	if len(req.ChannelIDs) > 0 {
		want := map[int64]bool{}
		for _, id := range req.ChannelIDs {
			want[id] = true
		}
		var filtered []db.PushChannel
		for _, c := range channels {
			if want[c.ID] {
				filtered = append(filtered, c)
			}
		}
		targets = filtered
	}
	var enabled []db.PushChannel
	for _, c := range targets {
		if c.Enabled == 1 {
			enabled = append(enabled, c)
		}
	}
	if len(enabled) == 0 {
		writeErr(w, 400, "没有可用的推送渠道，请先在设置中添加并启用")
		return
	}

	title := "🦜 鹦鹉情报站 · 每日安全简报 " + date
	type pushResult struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	results := []pushResult{}
	for _, ch := range enabled {
		full, err := s.DB.GetChannel(ch.ID)
		if err != nil {
			results = append(results, pushResult{ID: ch.ID, Name: ch.Name, Type: ch.Type, Error: "读取渠道失败"})
			continue
		}
		sender, err := notify.NewSender(full.Type, full.Webhook, full.Secret)
		if err != nil {
			results = append(results, pushResult{ID: ch.ID, Name: ch.Name, Type: ch.Type, Error: err.Error()})
			s.DB.UpdateChannelStatus(ch.ID, "error")
			continue
		}
		if err := sender.SendMarkdown(title, notify.TruncateMD(content, 18000)); err != nil {
			results = append(results, pushResult{ID: ch.ID, Name: ch.Name, Type: ch.Type, Error: err.Error()})
			s.DB.UpdateChannelStatus(ch.ID, "error: "+err.Error())
			continue
		}
		results = append(results, pushResult{ID: ch.ID, Name: ch.Name, Type: ch.Type, OK: true})
		s.DB.UpdateChannelStatus(ch.ID, "success")
	}
	writeJSON(w, map[string]any{"date": date, "results": results})
}

// ---------- push channels ----------

// GET /api/v1/channels
func (s *Server) apiListChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListChannels()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, list)
}

// POST /api/v1/channels {type,name,webhook,secret}
func (s *Server) apiAddChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Webhook string `json:"webhook"`
		Secret  string `json:"secret"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	if !db.ValidChannelTypes[req.Type] {
		writeErr(w, 400, "渠道类型必须是 dingtalk / wecom / feishu / serverchan")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Webhook) == "" {
		writeErr(w, 400, "名称和 Webhook 必填")
		return
	}
	id, err := s.DB.AddChannel(req.Type, req.Name, req.Webhook, req.Secret)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// PUT /api/v1/channels/{id} {name,webhook,secret?,enabled}
func (s *Server) apiUpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Name    string `json:"name"`
		Webhook string `json:"webhook"`
		Secret  string `json:"secret"`
		Enabled int    `json:"enabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Webhook) == "" {
		writeErr(w, 400, "名称和 Webhook 必填")
		return
	}
	if err := s.DB.UpdateChannel(id, req.Name, req.Webhook, req.Secret, req.Enabled); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// DELETE /api/v1/channels/{id}
func (s *Server) apiDeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.DB.DeleteChannel(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/channels/{id}/test —— 测试推送
func (s *Server) apiTestChannel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ch, err := s.DB.GetChannel(id)
	if err != nil {
		writeErr(w, 404, "渠道不存在")
		return
	}
	sender, err := notify.NewSender(ch.Type, ch.Webhook, ch.Secret)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := sender.SendMarkdown("🦜 鹦鹉情报站 · 测试消息", fmt.Sprintf("**%s 渠道连通性测试**\n\n如果你看到这条消息，说明「%s」渠道配置正确 ✅", notify.ChannelLabel(ch.Type), ch.Name)); err != nil {
		s.DB.UpdateChannelStatus(id, "error: "+err.Error())
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.DB.UpdateChannelStatus(id, "success")
	writeJSON(w, map[string]bool{"ok": true})
}

// ---------- helpers ----------

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

var _ = json.Marshal
