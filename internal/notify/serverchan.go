// Server酱（serverchan.com）微信推送。
package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ServerChan Server酱·Turbo（推送到个人微信「方糖」服务号）
type ServerChan struct {
	SendKey string // SendKey，如 SCT1234xxxx
}

// Configured 是否可用
func (s *ServerChan) Configured() bool { return s.SendKey != "" }

// SendMarkdown 推送（Server酱支持 markdown，标题取首行）
func (s *ServerChan) SendMarkdown(title, markdown string) error {
	if !s.Configured() {
		return fmt.Errorf("未配置 Server酱 SendKey")
	}
	// Server酱标题 ≤32，用首行兜底
	t := strings.TrimSpace(title)
	if t == "" {
		t = "每日安全简报"
	}
	if len([]rune(t)) > 32 {
		t = string([]rune(t)[:32])
	}

	api := "https://sctapi.ftqq.com/" + s.SendKey + ".send"
	form := url.Values{}
	form.Set("title", t)
	form.Set("desp", TruncateMD(markdown, 32000))

	resp, err := httpClient.PostForm(api, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Server酱 HTTP %d", resp.StatusCode)
	}
	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("Server酱响应解析失败: %v", err)
	}
	if r.Code != 0 {
		return fmt.Errorf("Server酱错误 %d: %s", r.Code, r.Msg)
	}
	return nil
}
