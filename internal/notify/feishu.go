// 飞书群机器人推送（支持签名校验）。
package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

// Feishu 飞书群机器人
type Feishu struct {
	Webhook string
	Secret  string // 签名密钥（可选）
}

// Configured 是否可用
func (f *Feishu) Configured() bool { return f.Webhook != "" }

func (f *Feishu) sign(t int64) string {
	// 飞书签名：timestamp + "\n" + secret 作为 HMAC-SHA256 的 key，对空串签名
	raw := fmt.Sprintf("%d\n%s", t, f.Secret)
	mac := hmac.New(sha256.New, []byte(raw))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// SendMarkdown 推送富文本（飞书用 post 格式；此处用 interactive 卡片 markdown）
func (f *Feishu) SendMarkdown(title, markdown string) error {
	if !f.Configured() {
		return fmt.Errorf("未配置飞书机器人 Webhook")
	}
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]string{"tag": "plain_text", "content": title},
				"template": "blue",
			},
			"elements": []map[string]any{
				{"tag": "markdown", "content": TruncateMD(markdown, 30000)},
			},
		},
	}
	if f.Secret != "" {
		t := time.Now().Unix()
		payload["timestamp"] = fmt.Sprintf("%d", t)
		payload["sign"] = f.sign(t)
	}

	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := postJSON(f.Webhook, payload, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("飞书错误 %d: %s", r.Code, r.Msg)
	}
	return nil
}
