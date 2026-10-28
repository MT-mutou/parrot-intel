// 企业微信群机器人推送。
package notify

import "fmt"

// WeCom 企业微信群机器人
type WeCom struct {
	Webhook string // https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx
}

// Configured 是否可用
func (w *WeCom) Configured() bool { return w.Webhook != "" }

// SendMarkdown 推送 markdown 消息（企微 markdown 上限 4096 字节，自动分段）
func (w *WeCom) SendMarkdown(title, markdown string) error {
	if !w.Configured() {
		return fmt.Errorf("未配置企业微信机器人 Webhook")
	}
	// 企微 markdown 单条上限 4096 字节，按段落分组发送
	chunks := splitMarkdownChunks(markdown, 3800)
	for i, chunk := range chunks {
		head := fmt.Sprintf("【%s】", title)
		if len(chunks) > 1 {
			head = fmt.Sprintf("【%s (%d/%d)】", title, i+1, len(chunks))
		}
		var r struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		err := postJSON(w.Webhook, map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": head + "\n" + chunk,
			},
		}, &r)
		if err != nil {
			return err
		}
		if r.ErrCode != 0 {
			return fmt.Errorf("企业微信错误 %d: %s", r.ErrCode, r.ErrMsg)
		}
	}
	return nil
}

// splitMarkdownChunks 按行边界把 markdown 切成 ≤max 字节的块
func splitMarkdownChunks(md string, max int) []string {
	if len(md) <= max {
		return []string{md}
	}
	var chunks []string
	lines := splitLines(md)
	var cur []byte
	for _, ln := range lines {
		if len(cur)+len(ln)+1 > max && len(cur) > 0 {
			chunks = append(chunks, string(cur))
			cur = nil
		}
		if len(cur) > 0 {
			cur = append(cur, '\n')
		}
		cur = append(cur, ln...)
	}
	if len(cur) > 0 {
		chunks = append(chunks, string(cur))
	}
	if len(chunks) == 0 {
		chunks = []string{md}
	}
	return chunks
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
