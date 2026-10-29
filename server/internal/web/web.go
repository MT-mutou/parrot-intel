// Package web 提供 RESTful API v1 纯后端服务。
// 前端（web/ 目录的 Vue3 工程）独立构建、由 Nginx 独立部署，
// 通过反向代理 /api 访问本服务；也支持跨域直连（CORS 可配置）。
package web

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/auth"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
)

// Version 服务版本号（/api/v1/health 与根路径返回）
const Version = "2.3.0"

// Server API 服务
type Server struct {
	DB   *db.DB
	Col  *collector.Collector
	Log  *log.Logger
	AI   func() *ai.Client // 每次调用实时读取设置（改配置无需重启）
	CORS string            // 允许的前端来源，逗号分隔；"*" 表示全部
	Auth *auth.Manager     // 登录会话管理
}

// New 构造
func New(d *db.DB, c *collector.Collector, aiFn func() *ai.Client, lg *log.Logger, cors string, am *auth.Manager) *Server {
	return &Server{DB: d, Col: c, Log: lg, AI: aiFn, CORS: cors, Auth: am}
}

// Handler 路由总入口（Go 1.22+ 方法路由）
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// ---- 服务信息与健康检查 ----
	mux.HandleFunc("GET /{$}", s.apiRoot)
	mux.HandleFunc("GET /api/v1/health", s.apiHealth)

	// ---- 登录认证（login 免鉴权，其余在中间件放行清单内）----
	mux.HandleFunc("POST /api/v1/auth/login", s.apiLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.apiLogout)
	mux.HandleFunc("GET /api/v1/auth/me", s.apiMe)
	mux.HandleFunc("POST /api/v1/auth/change-password", s.apiChangePassword)

	// ---- RESTful API v1 ----
	mux.HandleFunc("GET /api/v1/overview", s.apiOverview)
	mux.HandleFunc("GET /api/v1/articles", s.apiListArticles)
	mux.HandleFunc("GET /api/v1/articles/{id}", s.apiGetArticle)
	mux.HandleFunc("POST /api/v1/articles/{id}/star", s.apiStar)
	mux.HandleFunc("POST /api/v1/articles/{id}/read", s.apiRead)
	mux.HandleFunc("POST /api/v1/articles/{id}/analyze", s.apiAnalyze)
	mux.HandleFunc("GET /api/v1/stats", s.apiStats)

	mux.HandleFunc("GET /api/v1/sources", s.apiListSources)
	mux.HandleFunc("POST /api/v1/sources", s.apiAddSource)
	mux.HandleFunc("POST /api/v1/sources/{id}/toggle", s.apiToggleSource)
	mux.HandleFunc("DELETE /api/v1/sources/{id}", s.apiDeleteSource)

	mux.HandleFunc("GET /api/v1/settings", s.apiGetSettings)
	mux.HandleFunc("POST /api/v1/settings", s.apiSaveSettings)
	mux.HandleFunc("POST /api/v1/ai/models", s.apiAIModels)
	mux.HandleFunc("POST /api/v1/ai/crawler-config", s.apiAICrawlerConfig)

	mux.HandleFunc("POST /api/v1/collect", s.apiCollect)
	mux.HandleFunc("GET /api/v1/collect/status", s.apiCollectStatus)

	mux.HandleFunc("GET /api/v1/briefing", s.apiGetBriefing)
	mux.HandleFunc("GET /api/v1/briefings", s.apiBriefingList)
	mux.HandleFunc("GET /api/v1/briefing/profiles", s.apiBriefingProfileList)
	mux.HandleFunc("POST /api/v1/briefing/profiles", s.apiBriefingProfileAdd)
	mux.HandleFunc("PUT /api/v1/briefing/profiles/{id}", s.apiBriefingProfileUpdate)
	mux.HandleFunc("DELETE /api/v1/briefing/profiles/{id}", s.apiBriefingProfileDelete)
	mux.HandleFunc("POST /api/v1/briefing/generate", s.apiBriefingGenerate)
	mux.HandleFunc("POST /api/v1/briefing/push", s.apiBriefingPush)

	mux.HandleFunc("GET /api/v1/channels", s.apiListChannels)
	mux.HandleFunc("POST /api/v1/channels", s.apiAddChannel)
	mux.HandleFunc("PUT /api/v1/channels/{id}", s.apiUpdateChannel)
	mux.HandleFunc("DELETE /api/v1/channels/{id}", s.apiDeleteChannel)
	mux.HandleFunc("POST /api/v1/channels/{id}/test", s.apiTestChannel)

	return corsMiddleware(s.CORS, gzipMiddleware(s.authMiddleware(mux)))
}

// apiRoot 服务信息
func (s *Server) apiRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"service": "ParrotIntel API",
		"name":    "鹦鹉情报站",
		"version": Version,
		"api":     "/api/v1",
		"health":  "/api/v1/health",
	})
}

// apiHealth 健康检查（负载均衡 / 探活用）
func (s *Server) apiHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// ---------- JSON helpers ----------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	defer io.Copy(io.Discard, r.Body)
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// ---------- CORS ----------

// corsMiddleware 跨域支持：前端与 API 分开部署时（不同域名/端口）需要。
// 同域反向代理（Nginx 同一站点下 /api 反代）场景下无副作用。
func corsMiddleware(allow string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(allow, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = true
		}
	}
	wildcard := allowed["*"]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (wildcard || allowed[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "86400")
		}
		// 预检请求直接放行
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- gzip ----------

type gzipWriter struct {
	http.ResponseWriter
	gz   *gzip.Writer
	once sync.Once
}

// mark 确保在响应头实际发出前就声明 Content-Encoding。
// 关键：writeErr 会先 WriteHeader(code) 再写体；若 Content-Encoding
// 在第一次 Write 才补设，头部已定型，客户端会收到「gzip 体 + 无编码头」的乱码。
func (g *gzipWriter) mark() {
	g.once.Do(func() {
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Del("Content-Length")
	})
}

func (g *gzipWriter) WriteHeader(code int) {
	g.mark()
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	g.mark()
	return g.gz.Write(b)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipWriter{ResponseWriter: w, gz: gz}, r)
	})
}
