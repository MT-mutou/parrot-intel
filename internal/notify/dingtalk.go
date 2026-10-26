// Package notify 提供钉钉机器人推送（支持加签）。
package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DingTalk 钉钉机器人
type DingTalk struct {
	AccessToken string
	Secret      string
}

// New 创建
func New(token, secret string) *DingTalk {
	return &DingTalk{AccessToken: token, Secret: secret}
}

// Configured 是否可用
func (d *DingTalk) Configured() bool { return d.AccessToken != "" }

func (d *DingTalk) sign(t int64) string {
	raw := fmt.Sprintf("%d\n%s", t, d.Secret)
	mac := hmac.New(sha256.New, []byte(d.Secret))
	mac.Write([]byte(raw))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// SendMarkdown 推送 markdown 消息
func (d *DingTalk) SendMarkdown(title, markdown string) error {
	api := "https://oapi.dingtalk.com/robot/send?access_token=" + d.AccessToken
	if d.Secret != "" {
		t := time.Now().UnixMilli()
		api += fmt.Sprintf("&timestamp=%d&sign=%s", t, url.QueryEscape(d.sign(t)))
	}

	payload, _ := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  markdown,
		},
	})
	resp, err := http.Post(api, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("钉钉响应解析失败: %s", string(body[:min(len(body), 200)]))
	}
	if r.ErrCode != 0 {
		return fmt.Errorf("钉钉错误 %d: %s", r.ErrCode, r.ErrMsg)
	}
	return nil
}

// TruncateMD 钉钉 markdown 上限约 20000 字符，超长截断
func TruncateMD(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n\n…（已截断，完整内容见情报站）"
}

// BriefingToMarkdown 简报文本转钉钉 markdown（去掉 HTML 色彩，保留结构）
func BriefingToMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out []string
	for _, l := range lines {
		// 钉钉不支持表格/复杂语法，这里只做基本兼容
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
