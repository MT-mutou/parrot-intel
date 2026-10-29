// Package ai 提供 OpenAI 兼容的分析与简报生成。
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Config AI 配置
type Config struct {
	Endpoint string // 如 https://api.deepseek.com/v1
	APIKey   string
	Model    string // 如 deepseek-chat
}

// Analysis 单篇文章分析结果
type Analysis struct {
	Summary          string   `json:"summary"`
	Category         string   `json:"category"`
	Region           string   `json:"region"`
	ThreatLevel      string   `json:"threat_level"`
	Tags             []string `json:"tags"`
	PlainExplanation string   `json:"plain_explanation"`
}

// Client OpenAI 兼容客户端
type Client struct {
	cfg  Config
	http *http.Client
}

// New 创建客户端
func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}
}

// Configured 是否已配置可用
func (c *Client) Configured() bool { return c.cfg.APIKey != "" }

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) chat(system, user string) (string, error) {
	body, _ := json.Marshal(chatReq{
		Model: c.cfg.Model,
		Messages: []chatMsg{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.2,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(c.cfg.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI API %d: %s", resp.StatusCode, string(raw[:min(len(raw), 300)]))
	}
	var cr chatResp
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", err
	}
	if cr.Error != nil {
		return "", fmt.Errorf("AI API: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("AI API: 空响应")
	}
	return cr.Choices[0].Message.Content, nil
}

var jsonBlockRe = regexp.MustCompile("(?s)\\{.*\\}")

func extractJSON(s string) string {
	if m := jsonBlockRe.FindString(s); m != "" {
		return m
	}
	return s
}

// AnalyzeArticle 分析单篇文章：中文摘要 + 分类 + 威胁等级 + 标签 + 通俗解读
func (c *Client) AnalyzeArticle(title, content string) (*Analysis, error) {
	system := `你是网络安全情报分析师。分析给定的安全资讯，返回严格 JSON（不要 markdown 代码块）：
{"summary":"80-150字中文摘要","category":"分类","region":"国内/国际","threat_level":"high/medium/low","tags":["3-5个标签"],"plain_explanation":"一句通俗解读，让非技术人员明白影响"}
category 只能取：安全事件·漏洞 / 技术研究 / 政策法规 / 厂商动态 / AI安全 / 未分类
threat_level 判断：在野利用/勒索/数据泄露/高危漏洞=high；有补丁的漏洞/攻击事件=medium；其他=low`
	user := "标题：" + title + "\n\n正文（截断）：" + truncate(content, 6000)
	out, err := c.chat(system, user)
	if err != nil {
		return nil, err
	}
	var a Analysis
	if err := json.Unmarshal([]byte(extractJSON(out)), &a); err != nil {
		return nil, fmt.Errorf("解析 AI 输出失败: %w", err)
	}
	switch a.ThreatLevel {
	case "high", "medium", "low":
	default:
		a.ThreatLevel = ""
	}
	return &a, nil
}

// GenerateBriefing 生成每日简报（markdown，兼容旧调用：通用视角）
func (c *Client) GenerateBriefing(items []BriefingItem) (string, error) {
	p := BriefingProfile{
		Name:     "每日安全简报",
		Role:     "manager",
		Sections: []string{"overview", "vulnerabilities", "incidents", "actions"},
		Detail:   "normal",
	}
	return c.GenerateBriefingFor(time.Now().Format("2006-01-02"), items, p)
}

// BriefingItem 简报输入条目
type BriefingItem struct {
	Title    string
	Summary  string
	Threat   string
	Category string
}

// BriefingProfile 简报模板（角色化定制，与 db 层结构对齐但避免循环依赖）
type BriefingProfile struct {
	Name         string
	Role         string   // executive/manager/technical/compliance/custom
	Sections     []string // 模块 key 列表
	Detail       string   // brief/normal/detailed
	CustomPrompt string
}

// BriefingRoles 角色 → 读者画像描述
var BriefingRoles = map[string]string{
	"executive": "单位领导与决策层。关注宏观安全形势、政策监管趋势、对单位的整体风险影响与资源投入决策。避免技术细节与专有名词，语言凝练、结论先行，突出「领导需要知道什么、需要拍板什么」。",
	"manager":   "安全部门负责人与安全管理人员。关注漏洞与事件的风险等级、影响范围、处置优先级，以及需要协调的资源与跨部门行动，给出可落地的处置安排。",
	"technical": "一线安全技术与运营人员。保留 CVE 编号、受影响组件与版本、攻击手法等技术细节，给出具体修复方案、缓解措施与验证方式。",
	"compliance": "合规与风控岗位。关注政策法规、监管处罚、合规义务变化、数据泄露事件的合规影响，明确「哪些新要求需要落实、哪些事件涉及上报义务」。",
	"custom":    "（以模板的自定义读者描述为准）",
}

// BriefingSection 简报模块目录
type BriefingSection struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Hint  string `json:"hint"`
}

// BriefingSectionCatalog 全部可选模块（前端勾选框与后端 prompt 共用语义）
var BriefingSectionCatalog = []BriefingSection{
	{"overview", "当日形势总览", "一句话总结 + 整体风险态势（高/中/低危数量、类型分布）"},
	{"policy", "政策法规与监管", "国内外政策、法规、监管动态及其合规要求"},
	{"vulnerabilities", "高危漏洞与修复", "高危漏洞点名：编号、影响组件、修复建议"},
	{"incidents", "攻击与泄露事件", "攻击事件、勒索、数据泄露及其影响"},
	{"exploitation", "在野利用与威胁趋势", "已被在野利用的漏洞、活跃威胁组织与攻击趋势"},
	{"vendors", "厂商与产品动态", "安全厂商、产品发布与厂商安全公告"},
	{"ai", "AI 安全", "AI 模型安全、AI 相关攻击与防护动态"},
	{"research", "技术研究", "值得关注的攻击技术、分析研究"},
	{"actions", "处置建议与行动清单", "按优先级给出可执行动作"},
}

// BriefingDetails 详略程度 → 写作要求
var BriefingDetails = map[string]string{
	"brief":    "要点式：每个模块最多 5 条、每条一行，全文控制在 400 字内，适合快速浏览。",
	"normal":   "标准简报：每个模块 2-6 条要点，每条一句话概述 + 一句话影响/建议。",
	"detailed": "详尽展开：保留技术细节（CVE 编号、受影响版本、攻击手法），每条含影响分析与修复/缓解建议。",
}

func sectionTitle(key string) string {
	for _, s := range BriefingSectionCatalog {
		if s.Key == key {
			return s.Title
		}
	}
	return key
}

// GenerateBriefingFor 按模板（角色 + 模块 + 详略 + 自定义要求）生成简报
func (c *Client) GenerateBriefingFor(date string, items []BriefingItem, p BriefingProfile) (string, error) {
	roleDesc, ok := BriefingRoles[p.Role]
	if !ok {
		roleDesc = BriefingRoles["manager"]
	}
	detailDesc, ok := BriefingDetails[p.Detail]
	if !ok {
		detailDesc = BriefingDetails["normal"]
	}
	var sb strings.Builder
	sb.WriteString("你是安全运营中心的简报撰写人，为「" + p.Name + "」撰写 " + date + " 的安全简报。\n\n")
	sb.WriteString("读者：" + roleDesc + "\n")
	if p.Role == "custom" && p.CustomPrompt != "" {
		sb.WriteString("读者：" + p.CustomPrompt + "\n")
	} else if p.CustomPrompt != "" {
		sb.WriteString("其他要求：" + p.CustomPrompt + "\n")
	}
	sb.WriteString("\n输出 markdown：一级标题为「# 鹦鹉情报站 · " + p.Name + "（" + date + "）」。\n")
	sb.WriteString("然后严格按以下模块组织（二级标题用给定模块名，顺序一致；某模块当日无素材时写一句「今日暂无相关情报」，绝不编造）：\n")
	for _, key := range p.Sections {
		for _, s := range BriefingSectionCatalog {
			if s.Key == key {
				sb.WriteString("## " + s.Title + " —— " + s.Hint + "\n")
			}
		}
	}
	sb.WriteString("\n详略程度：" + detailDesc + "\n")
	sb.WriteString("写作要求：中文；克制专业；条目已标注威胁等级与分类，按模块相关性挑选，同一情报只放进最相关的一个模块；不编造条目中不存在的信息。\n")

	var list strings.Builder
	for i, it := range items {
		cat := it.Category
		if cat == "" {
			cat = "未分类"
		}
		list.WriteString(fmt.Sprintf("%d. [%s|%s] %s — %s\n", i+1, threatLabel(it.Threat), cat, it.Title, it.Summary))
	}
	out, err := c.chat(sb.String(), "当日情报条目：\n"+list.String())
	if err != nil {
		return "", err
	}
	return out, nil
}

// CrawlerConfig AI 生成的网页爬虫配置（与 collector.htmlConfig 字段对齐）
type CrawlerConfig struct {
	Item       string `json:"item"`
	Title      string `json:"title"`
	Link       string `json:"link"`
	LinkAttr   string `json:"link_attr"`
	Date       string `json:"date"`
	DateFormat string `json:"date_format"`
	Summary    string `json:"summary"`
}

// GenerateCrawlerConfig 分析列表页 HTML，生成 CSS 选择器爬虫配置。
// feedback 非空时附带上一轮失败信息，用于重试修正。
func (c *Client) GenerateCrawlerConfig(pageURL, html, feedback string) (*CrawlerConfig, error) {
	system := `你是网页结构分析专家。给定一个资讯列表页的 HTML，为通用爬虫生成 CSS 选择器配置。
返回严格 JSON（不要 markdown 代码块，不要解释）：
{"item":"列表中单条条目的选择器","title":"条目内标题元素的选择器","link":"条目内链接元素的选择器","link_attr":"链接属性名，一般是 href","date":"条目内时间元素的选择器，没有则为空字符串","date_format":"时间文本的格式，如 2006-01-02，不确定则为空字符串","summary":"条目内摘要/描述的选择器，没有则为空字符串"}
要求：
1. item 必须匹配到多条（列表条目），通常是最外层的重复单元（li、tr、带循环 class 的 div 等）
2. title/link 是相对 item 内部查找的选择器
3. 选择器要尽量稳健：优先语义化结构（ul li、table tr），避免随机哈希 class
4. 选择器语法为标准 CSS（goquery/cascadia 支持）`
	user := "页面地址：" + pageURL + "\n\nHTML（截断）：\n" + truncate(html, 24000)
	if feedback != "" {
		user += "\n\n注意：上一次生成的配置验证失败，请修正：\n" + feedback
	}
	out, err := c.chat(system, user)
	if err != nil {
		return nil, err
	}
	var cfg CrawlerConfig
	if err := json.Unmarshal([]byte(extractJSON(out)), &cfg); err != nil {
		return nil, fmt.Errorf("解析 AI 输出失败: %w", err)
	}
	if cfg.Item == "" || cfg.Title == "" || cfg.Link == "" {
		return nil, fmt.Errorf("AI 输出缺少 item/title/link 必填选择器")
	}
	if cfg.LinkAttr == "" {
		cfg.LinkAttr = "href"
	}
	return &cfg, nil
}

func threatLabel(t string) string {
	switch t {
	case "high":
		return "高危"
	case "medium":
		return "中危"
	case "low":
		return "低危"
	}
	return "未评"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
