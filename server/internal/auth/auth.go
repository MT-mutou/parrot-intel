// Package auth 提供登录会话管理与密码哈希。
// 凭据存放在 settings 表（admin_user / admin_pass_hash / force_change_pwd），
// 会话为内存态（服务重启后需重新登录，内部系统可接受）。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Session 一次登录会话
type Session struct {
	User    string
	Expires time.Time
}

// Manager 会话管理器
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager 构造
func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

// NewToken 生成 128bit 随机会话令牌
func NewToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Create 登录成功后建立会话
func (m *Manager) Create(user, token string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = &Session{User: user, Expires: time.Now().Add(ttl)}
}

// Valid 校验会话（滑动续期）
func (m *Manager) Valid(token string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || time.Now().After(s.Expires) {
		if ok {
			delete(m.sessions, token)
		}
		return "", false
	}
	s.Expires = time.Now().Add(7 * 24 * time.Hour)
	return s.User, true
}

// Remove 注销单个会话
func (m *Manager) Remove(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
}

// RemoveOthers 清除除 token 外的全部会话（修改密码后踢掉其他端）
func (m *Manager) RemoveOthers(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for t := range m.sessions {
		if t != token {
			delete(m.sessions, t)
		}
	}
}

// HashPassword bcrypt 哈希
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验密码与哈希
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
