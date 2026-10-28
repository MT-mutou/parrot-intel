// Package web 提供 HTTP 服务：Vue3 SPA（go:embed）+ RESTful API v1。
package web

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
)

//go:embed all:dist
var distFS embed.FS

// Server Web 服务
type Server struct {
	DB  *db.DB
	Col *collector.Collector
	Log *log.Logger
	AI  func() *ai.Client // 每次调用实时读取设置（改配置无需重启）
}

// New 构造
func New(d *db.DB, c *collector.Collector, aiFn func() *ai.Client, lg *log.Logger) *Server {
	return &Server{DB: d, Col: c, Log: lg, AI: aiFn}
}

// Handler 路由总入口（Go 1.22+ 方法路由）
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

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

	mux.HandleFunc("POST /api/v1/collect", s.apiCollect)
	mux.HandleFunc("GET /api/v1/collect/status", s.apiCollectStatus)

	mux.HandleFunc("GET /api/v1/briefing", s.apiGetBriefing)
	mux.HandleFunc("GET /api/v1/briefings", s.apiBriefingList)
	mux.HandleFunc("POST /api/v1/briefing/generate", s.apiBriefingGenerate)
	mux.HandleFunc("POST /api/v1/briefing/push", s.apiBriefingPush)

	mux.HandleFunc("GET /api/v1/channels", s.apiListChannels)
	mux.HandleFunc("POST /api/v1/channels", s.apiAddChannel)
	mux.HandleFunc("PUT /api/v1/channels/{id}", s.apiUpdateChannel)
	mux.HandleFunc("DELETE /api/v1/channels/{id}", s.apiDeleteChannel)
	mux.HandleFunc("POST /api/v1/channels/{id}/test", s.apiTestChannel)

	// ---- SPA 静态资源（go:embed dist）----
	mux.Handle("/", s.spaHandler())

	return gzipMiddleware(mux)
}

// spaHandler 服务 Vue 构建产物；未命中的路径回退 index.html（前端路由）
func (s *Server) spaHandler() http.Handler {
	sub, err := fsSub()
	if err != nil {
		panic("embed dist 目录缺失: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := sub.Open(p); err != nil {
			// SPA fallback：前端路由路径回退到 index.html
			data, _ := fsReadFile("index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		// 静态资源长缓存（Vite 产物带内容 hash）
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
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

// ---------- gzip ----------

type gzipWriter struct {
	http.ResponseWriter
	gz  *gzip.Writer
	once sync.Once
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	g.once.Do(func() {
		g.Header().Set("Content-Encoding", "gzip")
		g.Header().Set("Vary", "Accept-Encoding")
		g.Header().Del("Content-Length")
	})
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
