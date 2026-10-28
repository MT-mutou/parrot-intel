// Package notify 提供多渠道消息推送：钉钉 / 企业微信 / 飞书 / Server酱（微信）。
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Sender 渠道推送接口
type Sender interface {
	// SendMarkdown 推送 markdown 消息（title 为标题摘要，各渠道自行适配格式）
	SendMarkdown(title, markdown string) error
}

// NewSender 按渠道类型构造推送器
func NewSender(typ, webhook, secret string) (Sender, error) {
	switch typ {
	case "dingtalk":
		token := webhook
		// 兼容完整 webhook 地址或纯 token
		if idx := lastIndexOf(token, "access_token="); idx >= 0 {
			token = token[idx+len("access_token="):]
			if amp := firstIndexAny(token, "&"); amp >= 0 {
				token = token[:amp]
			}
		}
		return &DingTalk{AccessToken: token, Secret: secret}, nil
	case "wecom":
		return &WeCom{Webhook: webhook}, nil
	case "feishu":
		return &Feishu{Webhook: webhook, Secret: secret}, nil
	case "serverchan":
		return &ServerChan{SendKey: webhook}, nil
	default:
		return nil, fmt.Errorf("不支持的渠道类型: %s", typ)
	}
}

// ChannelLabel 渠道中文名
func ChannelLabel(typ string) string {
	switch typ {
	case "dingtalk":
		return "钉钉"
	case "wecom":
		return "企业微信"
	case "feishu":
		return "飞书"
	case "serverchan":
		return "微信（Server酱）"
	}
	return typ
}

func lastIndexOf(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func firstIndexAny(s, chars string) int {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return i
			}
		}
	}
	return -1
}

// TruncateMD markdown 上限保护（钉钉约 20000 字符），超长截断
func TruncateMD(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n\n…（已截断，完整内容见情报站）"
}

// postJSON 通用 JSON POST
func postJSON(url string, payload any, out any) error {
	body, _ := json.Marshal(payload)
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncStr(string(raw), 200))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("响应解析失败: %s", truncStr(string(raw), 200))
		}
	}
	return nil
}

func truncStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
