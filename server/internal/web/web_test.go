package web

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归测试：错误响应（writeErr 路径）经 gzip 中间件后，
// 响应头必须带 Content-Encoding: gzip，且解压后是可读 JSON。
// 修复前：WriteHeader 先于 Write 执行，Content-Encoding 未声明 → 客户端收到乱码。
func TestGzipErrorResponseReadable(t *testing.T) {
	s := &Server{CORS: "*"}
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/articles/deadbeef", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("error 响应缺少 Content-Encoding: gzip（实际 %q）—— 客户端会收到乱码", ce)
	}

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("响应体不是合法 gzip: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip 解压失败: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("解压后不是 JSON: %v, body=%q", err, string(raw))
	}
	if !strings.Contains(m["error"], "未登录") {
		t.Fatalf("error 字段内容异常: %q", m["error"])
	}
}

// 正常 JSON 响应路径同样要有正确的 gzip 头。
func TestGzipSuccessResponseReadable(t *testing.T) {
	s := &Server{CORS: "*"}
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("success 响应缺少 Content-Encoding: gzip（实际 %q）", ce)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("响应体不是合法 gzip: %v", err)
	}
	raw, _ := io.ReadAll(zr)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m["status"] != "ok" {
		t.Fatalf("health 响应异常: %v, body=%q", err, string(raw))
	}
}
