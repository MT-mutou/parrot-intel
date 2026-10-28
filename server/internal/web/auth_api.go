// 登录认证 API：登录 / 注销 / 会话信息 / 修改密码 + API 鉴权中间件。
package web

import (
	"net/http"
	"time"

	"parrot-intel/internal/auth"
)

const sessionCookie = "parrot_session"

// sessionTTL 会话有效期（滑动续期）
const sessionTTL = 7 * 24 * time.Hour

// ---------- 鉴权中间件 ----------

// publicAPI 无需登录即可访问的 API 路径
var publicAPI = map[string]bool{
	"/api/v1/health":    true,
	"/api/v1/auth/login": true,
}

// authMiddleware API 鉴权：未携带有效会话返回 401，前端跳转登录页
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicAPI[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.sessionUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		writeErr(w, http.StatusUnauthorized, "未登录或会话已过期")
	})
}

// sessionUser 从请求中解析会话用户
func (s *Server) sessionUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	return s.Auth.Valid(c.Value)
}

// ---------- handlers ----------

// POST /api/v1/auth/login {username, password}
func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeErr(w, 400, "请输入账号与密码")
		return
	}
	user := s.DB.GetSetting("admin_user", "yingwu")
	hash := s.DB.GetSetting("admin_pass_hash", "")
	if req.Username != user || !auth.CheckPassword(hash, req.Password) {
		// 轻微延迟，增加爆破成本
		time.Sleep(600 * time.Millisecond)
		writeErr(w, 401, "账号或密码错误")
		return
	}
	token := auth.NewToken()
	s.Auth.Create(user, token, sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL / time.Second),
	})
	writeJSON(w, map[string]any{
		"ok":                  true,
		"must_change_password": s.DB.GetSetting("force_change_pwd", "") == "true",
	})
}

// POST /api/v1/auth/logout
func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		s.Auth.Remove(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]bool{"ok": true})
}

// GET /api/v1/auth/me —— 会话检查（前端路由守卫用）
func (s *Server) apiMe(w http.ResponseWriter, r *http.Request) {
	user, ok := s.sessionUser(r)
	if !ok {
		writeErr(w, 401, "未登录")
		return
	}
	writeJSON(w, map[string]any{
		"username":            user,
		"must_change_password": s.DB.GetSetting("force_change_pwd", "") == "true",
	})
}

// POST /api/v1/auth/change-password {old_password, new_password}
func (s *Server) apiChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, 400, "请求体错误")
		return
	}
	if len(req.NewPassword) < 6 {
		writeErr(w, 400, "新密码长度至少 6 位")
		return
	}
	hash := s.DB.GetSetting("admin_pass_hash", "")
	if !auth.CheckPassword(hash, req.OldPassword) {
		writeErr(w, 400, "原密码错误")
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	if err := s.DB.SetSetting("admin_pass_hash", newHash); err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	s.DB.SetSetting("force_change_pwd", "false")
	// 修改密码后踢掉其他端（保留当前会话）
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		s.Auth.RemoveOthers(c.Value)
	}
	s.Log.Printf("[auth] %s 修改了密码", r.RemoteAddr)
	writeJSON(w, map[string]bool{"ok": true})
}
