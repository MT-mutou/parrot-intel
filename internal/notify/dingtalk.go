// 钉钉机器人推送（支持加签）。
package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
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

	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	err := postJSON(api, map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  markdown,
		},
	}, &r)
	if err != nil {
		return err
	}
	if r.ErrCode != 0 {
		return fmt.Errorf("钉钉错误 %d: %s", r.ErrCode, r.ErrMsg)
	}
	return nil
}
