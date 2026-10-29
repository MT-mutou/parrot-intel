// RESTful API v1 handlers。
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
	"parrot-intel/internal/notify"
)

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
		"articles":    arts,
		"total":       total,
		"page":        page,
		"limit":       limit,
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
		Type     string `json:"type"`
		Config   string `json:"config"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		writeErr(w, 400, "名称和 URL 必填")
		return
	}
	switch req.Type {
	case "", "rss", "html", "smartlink", "nvd", "ghsa", "qianxin":
	default:
		writeErr(w, 400, "不支持的来源类型: "+req.Type)
		return
	}
	if req.Type == "html" {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(req.Config), &cfg); err != nil {
			writeErr(w, 400, "爬虫配置不是合法 JSON: "+err.Error())
			return
		}
		if cfg["item"] == nil || cfg["title"] == nil || cfg["link"] == nil {
			writeErr(w, 400, "爬虫配置需包含 item / title / link 选择器")
			return
		}
	}
	if req.Type == "smartlink" {
		var cfg map[string]any
		if strings.TrimSpace(req.Config) != "" {
			if err := json.Unmarshal([]byte(req.Config), &cfg); err != nil {
				writeErr(w, 400, "智能链接配置不是合法 JSON: "+err.Error())
				return
			}
		}
		pat, _ := cfg["link_pattern"].(string)
		if strings.TrimSpace(pat) == "" {
			writeErr(w, 400, "智能链接配置需包含 link_pattern（条目链接特征，如 /news/detail）")
			return
		}
	}
	if err := s.DB.AddSource(req.Name, req.URL, req.Category, req.Type, req.Config); err != nil {
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
		"ai_endpoint":        get("ai_endpoint", "https://api.deepseek.com/v1"),
		"ai_model":           get("ai_model", "deepseek-chat"),
		"ai_api_key_set":     all["ai_api_key"] != "",
		"server_url":         all["server_url"],
		"collect_hour":       get("collect_hour", "9"),
		"auto_briefing_push": all["auto_briefing_push"] == "true",
		"collector_proxy":    all["collector_proxy"],
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
	// collector_proxy 允许显式清空（境内服务器不需要时）
	if v, ok := req["collector_proxy"]; ok {
		if pu := strings.TrimSpace(v); pu == "" || strings.HasPrefix(pu, "http://") || strings.HasPrefix(pu, "https://") {
			s.DB.SetSetting("collector_proxy", pu)
		}
	}
	if v, ok := req["auto_briefing_push"]; ok {
		s.DB.SetSetting("auto_briefing_push", v)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/ai/models {endpoint?, api_key?} —— 拉取 OpenAI 兼容端点的可用模型列表
// 请求体可选传端点/Key（用于保存前试连）；缺省回退到已保存的 AI 配置（用于验证已保存配置）。
// 成功即代表端点 + Key 可用。
func (s *Server) apiAIModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
		APIKey   string `json:"api_key"`
	}
	_ = decodeBody(r, &req) // 允许空请求体（全部回退到已保存配置）
	if ep := strings.TrimSpace(req.Endpoint); ep != "" {
		req.Endpoint = ep
	} else {
		req.Endpoint = s.DB.GetSetting("ai_endpoint", "https://api.deepseek.com/v1")
	}
	if k := strings.TrimSpace(req.APIKey); k != "" {
		req.APIKey = k
	} else {
		req.APIKey = s.DB.GetSetting("ai_api_key", "")
	}
	if req.APIKey == "" {
		writeErr(w, 400, "请先填写 API Key（或已在下方保存过）")
		return
	}

	client := &http.Client{Timeout: 20 * time.Second}
	hreq, err := http.NewRequest(http.MethodGet, strings.TrimRight(req.Endpoint, "/")+"/models", nil)
	if err != nil {
		writeErr(w, 400, "端点地址无效")
		return
	}
	hreq.Header.Set("Authorization", "Bearer "+req.APIKey)
	resp, err := client.Do(hreq)
	if err != nil {
		writeErr(w, 502, "连接端点失败："+err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		writeErr(w, 502, fmt.Sprintf("端点返回 %d：%s", resp.StatusCode, string(raw[:min(len(raw), 300)])))
		return
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Data) == 0 {
		writeErr(w, 502, "端点未返回模型列表——请确认是 OpenAI 兼容端点（一般以 /v1 结尾）且 Key 有效")
		return
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	writeJSON(w, map[string]any{"models": ids, "endpoint": req.Endpoint})
}

// POST /api/v1/ai/crawler-config {url} —— AI 分析列表页结构，自动生成网页爬虫的 CSS 选择器配置
// 生成后会用真实页面试跑验证；首次未命中会带失败反馈重试一次。
func (s *Server) apiAICrawlerConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeBody(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
		writeErr(w, 400, "请填写列表页 URL")
		return
	}
	pageURL := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(pageURL, "http://") && !strings.HasPrefix(pageURL, "https://") {
		writeErr(w, 400, "URL 需以 http:// 或 https:// 开头")
		return
	}
	c := s.AI()
	if !c.Configured() {
		writeErr(w, 400, "未配置 AI API Key，请先到「系统设置」配置 AI 分析引擎")
		return
	}

	html, err := s.Col.FetchPageHTML(pageURL)
	if err != nil {
		writeErr(w, 502, "抓取页面失败："+err.Error())
		return
	}

	// SPA 空壳预检：Vue/React 单页应用的服务端 HTML 没有列表数据，AI 必然生成不出选择器。
	// 提前给出针对性指引，避免用户浪费一轮 AI 调用后只看到晦涩的报错。
	if looksLikeSPA(html) {
		writeErr(w, 400, "该页面是 JS 动态渲染的单页应用（Vue/React），服务器抓到的 HTML 只有空壳、没有列表数据，无法用 CSS 选择器爬取。\n"+
			"解决方案：① 奇安信威胁情报中心（ti.qianxin.com）已内置专用类型——添加来源时类型选「奇安信 TI」即可；"+
			"② 其他站点请在浏览器按 F12 → Network → XHR 找到返回列表数据的 JSON 接口，或寻找其 RSS 地址。")
		return
	}

	var cfg *ai.CrawlerConfig
	var matched int
	var samples []collector.CrawlerSample
	feedback := ""
	for attempt := 0; attempt < 2; attempt++ {
		cfg, err = c.GenerateCrawlerConfig(pageURL, html, feedback)
		if err != nil {
			msg := "AI 生成配置失败：" + err.Error()
			if strings.Contains(err.Error(), "必填选择器") {
				msg += "\n页面可能缺少静态列表结构（如 JS 动态渲染）：奇安信 TI 等站点请改用内置专用类型，「来源类型」选「奇安信 TI」"
			}
			writeErr(w, 502, msg)
			return
		}
		cfgJSON, _ := json.Marshal(cfg)
		matched, samples, err = collector.ProbeCrawlerConfig(html, string(cfgJSON), pageURL)
		if err == nil && matched > 0 && len(samples) > 0 {
			break
		}
		feedback = fmt.Sprintf("选择器 %q 在页面中匹配到 %d 个条目（需 >0 且能提取出标题与链接），请更换更稳健的选择器",
			cfg.Item, matched)
	}
	if matched == 0 || len(samples) == 0 {
		writeErr(w, 502, "AI 生成的选择器未能从页面提取到条目（匹配 "+strconv.Itoa(matched)+" 条），该页面可能是 JS 动态渲染，请手工配置或换用 RSS/API 源")
		return
	}
	writeJSON(w, map[string]any{
		"config":  cfg,
		"matched": matched,
		"samples": samples,
	})
}

// looksLikeSPA 判断页面 HTML 是否为 JS 单页应用空壳：
// 特征为存在 <div id="app">/<div id="root"> 挂载点，且去除脚本与标签后可见文本极少。
func looksLikeSPA(html string) bool {
	lower := strings.ToLower(html)
	hasMount := strings.Contains(lower, `id="app"`) || strings.Contains(lower, `id="root"`) ||
		strings.Contains(lower, `id='app'`) || strings.Contains(lower, `id='root'`)
	if !hasMount {
		return false
	}
	text := scriptStyleRe.ReplaceAllString(html, "")
	text = tagRe.ReplaceAllString(text, " ")
	return len(strings.Fields(text)) < 200
}

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]*>`)
)

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

// aiProfile db 模板 → ai 模板
func aiProfile(p *db.BriefingProfile) ai.BriefingProfile {
	return ai.BriefingProfile{
		Name:         p.Name,
		Role:         p.Role,
		Sections:     p.Sections,
		Detail:       p.Detail,
		CustomPrompt: p.CustomPrompt,
	}
}

// parseProfileID 从查询参数取 profile_id（缺省/非法 → 0 通用简报，兼容旧数据）
func parseProfileID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.URL.Query().Get("profile_id"), 10, 64)
	return id
}

// GET /api/v1/briefing/profiles —— 简报模板列表（含角色/模块目录供前端渲染）
func (s *Server) apiBriefingProfileList(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListBriefingProfiles()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"profiles":  list,
		"roles":     ai.BriefingRoles,
		"sections":  ai.BriefingSectionCatalog,
		"details":   []string{"brief", "normal", "detailed"},
	})
}

// POST /api/v1/briefing/profiles {name,role,sections,detail,custom_prompt,enabled?}
func (s *Server) apiBriefingProfileAdd(w http.ResponseWriter, r *http.Request) {
	p, err := decodeBriefingProfile(r, 0)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, err := s.DB.AddBriefingProfile(p)
	if err != nil {
		writeErr(w, 500, "保存失败："+err.Error())
		return
	}
	p.ID = id
	writeJSON(w, p)
}

// PUT /api/v1/briefing/profiles/{id}
func (s *Server) apiBriefingProfileUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "ID 非法")
		return
	}
	if _, err := s.DB.GetBriefingProfile(id); err != nil {
		writeErr(w, 404, "模板不存在")
		return
	}
	p, err := decodeBriefingProfile(r, id)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.DB.UpdateBriefingProfile(p); err != nil {
		writeErr(w, 500, "保存失败："+err.Error())
		return
	}
	writeJSON(w, p)
}

// DELETE /api/v1/briefing/profiles/{id}
func (s *Server) apiBriefingProfileDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "ID 非法")
		return
	}
	if err := s.DB.DeleteBriefingProfile(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// decodeBriefingProfile 解析并校验模板请求体
func decodeBriefingProfile(r *http.Request, id int64) (*db.BriefingProfile, error) {
	var req struct {
		Name         string   `json:"name"`
		Role         string   `json:"role"`
		Sections     []string `json:"sections"`
		Detail       string   `json:"detail"`
		CustomPrompt string   `json:"custom_prompt"`
		Enabled      *int     `json:"enabled"`
	}
	if err := decodeBody(r, &req); err != nil {
		return nil, fmt.Errorf("请求体错误")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("模板名称必填")
	}
	if !db.ValidBriefingRoles[req.Role] {
		return nil, fmt.Errorf("角色必须是 executive / manager / technical / compliance / custom")
	}
	if !db.ValidBriefingDetails[req.Detail] {
		return nil, fmt.Errorf("详略程度必须是 brief / normal / detailed")
	}
	if req.Role == "custom" && strings.TrimSpace(req.CustomPrompt) == "" {
		return nil, fmt.Errorf("自定义角色需填写读者描述")
	}
	// 模块 key 白名单过滤（保持用户勾选顺序）
	valid := map[string]bool{}
	for _, s := range ai.BriefingSectionCatalog {
		valid[s.Key] = true
	}
	var secs []string
	for _, k := range req.Sections {
		if valid[k] {
			secs = append(secs, k)
		}
	}
	if len(secs) == 0 {
		return nil, fmt.Errorf("至少选择一个内容模块")
	}
	enabled := 1
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return &db.BriefingProfile{
		ID: id, Name: req.Name, Role: req.Role, Sections: secs,
		Detail: req.Detail, CustomPrompt: strings.TrimSpace(req.CustomPrompt), Enabled: enabled,
	}, nil
}

// GET /api/v1/briefing?date=2006-01-02&profile_id=1
func (s *Server) apiGetBriefing(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	content, err := s.DB.GetBriefing(date, parseProfileID(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"date": date, "profile_id": parseProfileID(r), "content": content})
}

// GET /api/v1/briefings?profile_id=1 —— 历史简报列表（按模板过滤）
func (s *Server) apiBriefingList(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListBriefingDates(parseProfileID(r), 60)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, list)
}

// POST /api/v1/briefing/generate {date?, profile_id?}
func (s *Server) apiBriefingGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date      string `json:"date"`
		ProfileID int64  `json:"profile_id"`
	}
	_ = decodeBody(r, &req)
	date := req.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	// profile_id=0：兼容旧版通用简报（无模板定义时使用默认管理视角）
	var prof ai.BriefingProfile
	if req.ProfileID > 0 {
		p, err := s.DB.GetBriefingProfile(req.ProfileID)
		if err != nil {
			writeErr(w, 404, "简报模板不存在")
			return
		}
		if p.Enabled != 1 {
			writeErr(w, 400, "模板已停用，请先在模板管理中启用")
			return
		}
		prof = aiProfile(p)
	} else {
		prof = ai.BriefingProfile{Name: "每日安全简报", Role: "manager",
			Sections: []string{"overview", "vulnerabilities", "incidents", "actions"}, Detail: "normal"}
	}

	items, err := s.DB.ArticlesOnDate(date)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(items) == 0 {
		writeErr(w, 400, date+" 当日暂无文章，请先收集")
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
		cat := ""
		if it.Category != nil {
			cat = *it.Category
		}
		bis = append(bis, ai.BriefingItem{Title: it.Title, Summary: sum, Threat: threat, Category: cat})
	}
	md, err := c.GenerateBriefingFor(date, bis, prof)
	if err != nil {
		writeErr(w, 502, "简报生成失败："+err.Error())
		return
	}
	s.DB.SaveBriefing(date, req.ProfileID, md)
	writeJSON(w, map[string]any{"date": date, "profile_id": req.ProfileID, "content": md})
}

// POST /api/v1/briefing/push {date?, profile_id?, channel_ids?}
func (s *Server) apiBriefingPush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date       string  `json:"date"`
		ProfileID  int64   `json:"profile_id"`
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
	content, _ := s.DB.GetBriefing(date, req.ProfileID)
	if content == "" {
		writeErr(w, 400, "当日该模板简报尚未生成，请先生成")
		return
	}
	// 推送标题带模板名
	profName := "每日安全简报"
	if req.ProfileID > 0 {
		if p, err := s.DB.GetBriefingProfile(req.ProfileID); err == nil {
			profName = p.Name
		}
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

	title := "🦜 鹦鹉情报站 · " + profName + " " + date
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
