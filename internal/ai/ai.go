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
	Summary         string   `json:"summary"`
	Category        string   `json:"category"`
	Region          string   `json:"region"`
	ThreatLevel     string   `json:"threat_level"`
	Tags            []string `json:"tags"`
	PlainExplanation string  `json:"plain_explanation"`
}

// Client OpenAI 兼容客户端
type Client struct {
	cfg    Config
	http   *http.Client
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

// GenerateBriefing 生成每日简报（markdown）
func (c *Client) GenerateBriefing(items []BriefingItem) (string, error) {
	system := `你是安全运营中心的简报撰写人。根据当日情报条目生成 markdown 每日安全简报，结构：
# 鹦鹉情报站 · 每日安全简报
## 一句话总结
## 重点威胁（高危条目点名，含影响与建议动作）
## 值得关注（中低危要点，合并同类）
## 建议行动（3-5条可执行清单）
面向管理层与安全团队，中文，克制专业，不编造条目中不存在的信息。`
	var sb strings.Builder
	for i, it := range items {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s — %s\n", i+1, threatLabel(it.Threat), it.Title, it.Summary))
	}
	out, err := c.chat(system, "当日情报条目：\n"+sb.String())
	if err != nil {
		return "", err
	}
	return out, nil
}

// BriefingItem 简报输入条目
type BriefingItem struct {
	Title   string
	Summary string
	Threat  string
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
