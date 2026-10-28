// Package db 提供 SQLite 持久化：文章、来源、设置。
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	mu sync.Mutex // better 并发保护（modernc sqlite 单写者）
)

// DB 全局数据库句柄
type DB struct {
	sql *sql.DB
}

// Open 打开（或初始化）数据库
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "parrot.db")
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1) // modernc sqlite 写并发安全优先
	db := &DB{sql: d}
	if err := db.init(); err != nil {
		return nil, err
	}
	return db, nil
}

func (d *DB) init() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS articles (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  source_name TEXT,
  source_url TEXT,
  summary TEXT,
  content TEXT,
  category TEXT,
  threat_level TEXT,
  region TEXT,
  tags TEXT,
  cve_ids TEXT,
  plain_explanation TEXT,
  published_at TEXT,
  collected_at TEXT,
  is_read INTEGER DEFAULT 0,
  is_starred INTEGER DEFAULT 0,
  ai_analyzed INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_art_published ON articles(published_at);
CREATE INDEX IF NOT EXISTS idx_art_category ON articles(category);
CREATE INDEX IF NOT EXISTS idx_art_threat  ON articles(threat_level);
CREATE INDEX IF NOT EXISTS idx_art_starred ON articles(is_starred);
CREATE INDEX IF NOT EXISTS idx_art_collected ON articles(collected_at);

CREATE TABLE IF NOT EXISTS sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  category TEXT,
  enabled INTEGER DEFAULT 1,
  last_fetch_at TEXT,
  last_status TEXT,
  last_error TEXT,
  fail_count INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT
);

CREATE TABLE IF NOT EXISTS briefings (
  date TEXT PRIMARY KEY,
  content TEXT,
  generated_at TEXT
);

CREATE TABLE IF NOT EXISTS push_channels (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,           -- dingtalk / wecom / feishu / serverchan
  name TEXT NOT NULL,
  webhook TEXT,                 -- 机器人 Webhook 地址 / Server酱 SendKey
  secret TEXT,                  -- 钉钉加签 / 飞书签名密钥
  enabled INTEGER DEFAULT 1,
  created_at TEXT,
  last_push_at TEXT,
  last_status TEXT
);
`)
	return err
}

// ---------- Push Channels ----------

// PushChannel 推送渠道
type PushChannel struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Name       string  `json:"name"`
	Webhook    string  `json:"webhook"`
	Secret     string  `json:"secret,omitempty"` // 不回传完整 secret
	SecretSet  bool    `json:"secret_set"`
	Enabled    int     `json:"enabled"`
	LastPushAt *string `json:"last_push_at"`
	LastStatus *string `json:"last_status"`
}

// ValidChannelTypes 支持的渠道类型
var ValidChannelTypes = map[string]bool{
	"dingtalk": true, "wecom": true, "feishu": true, "serverchan": true,
}

// ListChannels 全部推送渠道
func (d *DB) ListChannels() ([]PushChannel, error) {
	rows, err := d.sql.Query(`SELECT id, type, name, webhook, secret, enabled, last_push_at, last_status FROM push_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []PushChannel{}
	for rows.Next() {
		var c PushChannel
		var secret sql.NullString
		if err := rows.Scan(&c.ID, &c.Type, &c.Name, &c.Webhook, &secret, &c.Enabled, &c.LastPushAt, &c.LastStatus); err != nil {
			return nil, err
		}
		c.SecretSet = secret.Valid && secret.String != ""
		list = append(list, c)
	}
	return list, nil
}

// GetChannel 按 ID 取渠道（含完整 secret，仅内部使用）
func (d *DB) GetChannel(id int64) (*PushChannel, error) {
	var c PushChannel
	var secret sql.NullString
	err := d.sql.QueryRow(`SELECT id, type, name, webhook, secret, enabled, last_push_at, last_status FROM push_channels WHERE id=?`, id).
		Scan(&c.ID, &c.Type, &c.Name, &c.Webhook, &secret, &c.Enabled, &c.LastPushAt, &c.LastStatus)
	if err != nil {
		return nil, err
	}
	if secret.Valid {
		c.Secret = secret.String
		c.SecretSet = true
	}
	return &c, nil
}

// AddChannel 新增渠道
func (d *DB) AddChannel(typ, name, webhook, secret string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	res, err := d.sql.Exec(
		`INSERT INTO push_channels (type, name, webhook, secret, enabled, created_at) VALUES (?, ?, ?, ?, 1, ?)`,
		typ, name, nullIfEmpty(webhook), nullIfEmpty(secret), time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateChannel 更新渠道（空 secret = 保持不变）
func (d *DB) UpdateChannel(id int64, name, webhook, secret string, enabled int) error {
	mu.Lock()
	defer mu.Unlock()
	if secret != "" {
		_, err := d.sql.Exec(`UPDATE push_channels SET name=?, webhook=?, secret=?, enabled=? WHERE id=?`,
			name, nullIfEmpty(webhook), secret, enabled, id)
		return err
	}
	_, err := d.sql.Exec(`UPDATE push_channels SET name=?, webhook=?, enabled=? WHERE id=?`,
		name, nullIfEmpty(webhook), enabled, id)
	return err
}

// DeleteChannel 删除渠道
func (d *DB) DeleteChannel(id int64) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`DELETE FROM push_channels WHERE id=?`, id)
	return err
}

// UpdateChannelStatus 记录推送结果
func (d *DB) UpdateChannelStatus(id int64, status string) {
	mu.Lock()
	defer mu.Unlock()
	d.sql.Exec(`UPDATE push_channels SET last_push_at=?, last_status=? WHERE id=?`,
		time.Now().Format(time.RFC3339), status, id)
}

// EnabledChannels 启用的渠道（推送用，含 secret）
func (d *DB) EnabledChannels() ([]PushChannel, error) {
	rows, err := d.sql.Query(`SELECT id, type, name, webhook, secret, enabled, last_push_at, last_status FROM push_channels WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []PushChannel{}
	for rows.Next() {
		var c PushChannel
		var secret sql.NullString
		if err := rows.Scan(&c.ID, &c.Type, &c.Name, &c.Webhook, &secret, &c.Enabled, &c.LastPushAt, &c.LastStatus); err != nil {
			return nil, err
		}
		if secret.Valid {
			c.Secret = secret.String
		}
		list = append(list, c)
	}
	return list, nil
}

// ListBriefingDates 简报日期列表（历史简报）
func (d *DB) ListBriefingDates(limit int) ([]map[string]string, error) {
	rows, err := d.sql.Query(`SELECT date, generated_at FROM briefings ORDER BY date DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []map[string]string{}
	for rows.Next() {
		var date string
		var gen sql.NullString
		if err := rows.Scan(&date, &gen); err == nil {
			list = append(list, map[string]string{"date": date, "generated_at": gen.String})
		}
	}
	return list, nil
}

// ---------- Article ----------

// Article 文章（列表查询不含 content）
type Article struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	URL             string  `json:"url"`
	SourceName      string  `json:"source_name"`
	SourceURL       string  `json:"source_url"`
	Summary         *string `json:"summary"`
	Category        *string `json:"category"`
	ThreatLevel     *string `json:"threat_level"`
	Region          *string `json:"region"`
	Tags            []string `json:"tags"`
	CVEIDs          []string `json:"cve_ids"`
	PlainExplanation *string `json:"plain_explanation"`
	PublishedAt     *string `json:"published_at"`
	IsRead          int     `json:"is_read"`
	IsStarred       int     `json:"is_starred"`
	AIAnalyzed      int     `json:"ai_analyzed"`
	// Content 仅详情查询填充，列表查询不取（性能关键）
	Content string `json:"content,omitempty"`
}

// ArticleDetail 详情（含入库时间）
type ArticleDetail struct {
	Article
	CollectedAt string `json:"collected_at"`
}

// ListFilter 列表过滤条件
type ListFilter struct {
	Page     int
	Limit    int
	Category string
	Threat   string
	Search   string
	Source   string
	Starred  bool
	Unread   bool
}

const listCols = `id, title, url, source_name, source_url, summary, category, threat_level, region, tags, cve_ids, plain_explanation, published_at, is_read, is_starred, ai_analyzed`

func scanArticle(scan func(...any) error, withContent bool) (*Article, error) {
	var a Article
	var tags, cves sql.NullString
	dest := []any{&a.ID, &a.Title, &a.URL, &a.SourceName, &a.SourceURL, &a.Summary,
		&a.Category, &a.ThreatLevel, &a.Region, &tags, &cves, &a.PlainExplanation,
		&a.PublishedAt, &a.IsRead, &a.IsStarred, &a.AIAnalyzed}
	if withContent {
		dest = append(dest, &a.Content)
	}
	if err := scan(dest...); err != nil {
		return nil, err
	}
	a.Tags = parseJSONArr(tags)
	a.CVEIDs = parseJSONArr(cves)
	return &a, nil
}

func parseJSONArr(s sql.NullString) []string {
	if !s.Valid || s.String == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s.String), &arr); err != nil {
		return nil
	}
	return arr
}

// ListArticles 分页列表（不查 content，极快）
func (d *DB) ListArticles(f ListFilter) ([]Article, int, error) {
	where := "1=1"
	args := []any{}
	if f.Category != "" && f.Category != "all" {
		where += " AND category = ?"
		args = append(args, f.Category)
	}
	if f.Threat != "" && f.Threat != "all" {
		where += " AND threat_level = ?"
		args = append(args, f.Threat)
	}
	if f.Source != "" {
		where += " AND source_name = ?"
		args = append(args, f.Source)
	}
	if f.Starred {
		where += " AND is_starred = 1"
	}
	if f.Unread {
		where += " AND is_read = 0"
	}
	if f.Search != "" {
		where += " AND (title LIKE ? OR summary LIKE ?)"
		term := "%" + f.Search + "%"
		args = append(args, term, term)
	}

	var total int
	if err := d.sql.QueryRow("SELECT COUNT(*) FROM articles WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 25
	}
	offset := (f.Page - 1) * f.Limit

	rows, err := d.sql.Query(
		"SELECT "+listCols+" FROM articles WHERE "+where+" ORDER BY published_at DESC LIMIT ? OFFSET ?",
		append(args, f.Limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Article{}
	for rows.Next() {
		a, err := scanArticle(rows.Scan, false)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *a)
	}
	return list, total, nil
}

// GetArticle 详情（含全文）
func (d *DB) GetArticle(id string) (*ArticleDetail, error) {
	var ad ArticleDetail
	var tags, cves sql.NullString
	err := d.sql.QueryRow(
		`SELECT id, title, url, source_name, source_url, summary, category, threat_level, region, tags, cve_ids, plain_explanation, published_at, is_read, is_starred, ai_analyzed, content, collected_at FROM articles WHERE id = ?`, id).
		Scan(&ad.ID, &ad.Title, &ad.URL, &ad.SourceName, &ad.SourceURL, &ad.Summary,
			&ad.Category, &ad.ThreatLevel, &ad.Region, &tags, &cves, &ad.PlainExplanation,
			&ad.PublishedAt, &ad.IsRead, &ad.IsStarred, &ad.AIAnalyzed, &ad.Content, &ad.CollectedAt)
	if err != nil {
		return nil, err
	}
	ad.Tags = parseJSONArr(tags)
	ad.CVEIDs = parseJSONArr(cves)
	return &ad, nil
}

// InsertArticle 新文章入库（URL 去重），返回是否新插入
func (d *DB) InsertArticle(id, title, url, sourceName, sourceURL, summary, content, publishedAt string) bool {
	mu.Lock()
	defer mu.Unlock()
	res, err := d.sql.Exec(
		`INSERT OR IGNORE INTO articles (id, title, url, source_name, source_url, summary, content, published_at, collected_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, url, sourceName, sourceURL, nullIfEmpty(summary), nullIfEmpty(content),
		nullIfEmpty(publishedAt), time.Now().Format(time.RFC3339))
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// UpdateAnalysis 写回 AI 分析结果
func (d *DB) UpdateAnalysis(id string, summary, category, region, threat string, tags, cves []string, plain string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(
		`UPDATE articles SET summary=?, category=?, region=?, threat_level=?, tags=?, cve_ids=?, plain_explanation=?, ai_analyzed=1 WHERE id=?`,
		summary, nullIfEmpty(category), nullIfEmpty(region), nullIfEmpty(threat),
		toJSONArr(tags), toJSONArr(cves), nullIfEmpty(plain), id)
	return err
}

// UnanalyzedIDs 取待 AI 分析的文章（id + title + summary + content）
func (d *DB) Unanalyzed(limit int) ([]ArticleDetail, error) {
	rows, err := d.sql.Query(
		`SELECT id, title, url, source_name, source_url, summary, category, threat_level, region, tags, cve_ids, plain_explanation, published_at, is_read, is_starred, ai_analyzed, content, collected_at
		 FROM articles WHERE ai_analyzed = 0 ORDER BY collected_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []ArticleDetail{}
	for rows.Next() {
		var ad ArticleDetail
		var tags, cves sql.NullString
		if err := rows.Scan(&ad.ID, &ad.Title, &ad.URL, &ad.SourceName, &ad.SourceURL, &ad.Summary,
			&ad.Category, &ad.ThreatLevel, &ad.Region, &tags, &cves, &ad.PlainExplanation,
			&ad.PublishedAt, &ad.IsRead, &ad.IsStarred, &ad.AIAnalyzed, &ad.Content, &ad.CollectedAt); err != nil {
			return nil, err
		}
		ad.Tags = parseJSONArr(tags)
		ad.CVEIDs = parseJSONArr(cves)
		list = append(list, ad)
	}
	return list, nil
}

// ToggleStar 切换收藏
func (d *DB) ToggleStar(id string) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`UPDATE articles SET is_starred = 1 - is_starred WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	var s int
	d.sql.QueryRow(`SELECT is_starred FROM articles WHERE id=?`, id).Scan(&s)
	return s == 1, nil
}

// MarkRead 标记已读
func (d *DB) MarkRead(id string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`UPDATE articles SET is_read = 1 WHERE id = ?`, id)
	return err
}

// TodayArticles 简报用：当日文章摘要列表
func (d *DB) TodayArticles() ([]Article, error) {
	rows, err := d.sql.Query(
		`SELECT `+listCols+` FROM articles
		 WHERE date(collected_at) = date('now','localtime')
		 ORDER BY CASE threat_level WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, published_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Article{}
	for rows.Next() {
		a, err := scanArticle(rows.Scan, false)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, nil
}

// Stats 仪表盘统计
type Stats struct {
	Total      int            `json:"total"`
	Today      int            `json:"today"`
	Unread     int            `json:"unread"`
	Starred    int            `json:"starred"`
	HighThreat int            `json:"high_threat"`
	ByCategory []CategoryStat `json:"by_category"`
	BySource   []CategoryStat `json:"by_source"`
	ByThreat   []CategoryStat `json:"by_threat"`
	Last7Days  []DayStat      `json:"last_7days"`
}

// CategoryStat 分类统计
type CategoryStat struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// DayStat 每日数量
type DayStat struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// GetStats 统计
func (d *DB) GetStats() (*Stats, error) {
	s := &Stats{}
	d.sql.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&s.Total)
	d.sql.QueryRow(`SELECT COUNT(*) FROM articles WHERE date(collected_at)=date('now','localtime')`).Scan(&s.Today)
	d.sql.QueryRow(`SELECT COUNT(*) FROM articles WHERE is_read=0`).Scan(&s.Unread)
	d.sql.QueryRow(`SELECT COUNT(*) FROM articles WHERE is_starred=1`).Scan(&s.Starred)
	d.sql.QueryRow(`SELECT COUNT(*) FROM articles WHERE threat_level='high'`).Scan(&s.HighThreat)

	rows, err := d.sql.Query(`SELECT COALESCE(category,'未分类') cat, COUNT(*) FROM articles GROUP BY cat ORDER BY 2 DESC LIMIT 8`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CategoryStat
			rows.Scan(&c.Category, &c.Count)
			s.ByCategory = append(s.ByCategory, c)
		}
	}
	rows3, err := d.sql.Query(`SELECT COALESCE(source_name,'未知') src, COUNT(*) FROM articles GROUP BY src ORDER BY 2 DESC LIMIT 10`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var c CategoryStat
			rows3.Scan(&c.Category, &c.Count)
			s.BySource = append(s.BySource, c)
		}
	}
	rows4, err := d.sql.Query(`SELECT COALESCE(threat_level,'未评') t, COUNT(*) FROM articles GROUP BY t`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var c CategoryStat
			rows4.Scan(&c.Category, &c.Count)
			s.ByThreat = append(s.ByThreat, c)
		}
	}
	rows2, err := d.sql.Query(`SELECT date(collected_at) d, COUNT(*) FROM articles WHERE collected_at >= datetime('now','localtime','-7 days') GROUP BY d ORDER BY d`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var ds DayStat
			rows2.Scan(&ds.Day, &ds.Count)
			s.Last7Days = append(s.Last7Days, ds)
		}
	}
	return s, nil
}

// ---------- Sources ----------

// Source RSS 来源
type Source struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	Category   *string `json:"category"`
	Enabled    int     `json:"enabled"`
	LastFetch  *string `json:"last_fetch_at"`
	LastStatus *string `json:"last_status"`
	LastError  *string `json:"last_error"`
}

// ListSources 全部来源
func (d *DB) ListSources() ([]Source, error) {
	rows, err := d.sql.Query(`SELECT id, name, url, category, enabled, last_fetch_at, last_status, last_error FROM sources ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.URL, &s.Category, &s.Enabled, &s.LastFetch, &s.LastStatus, &s.LastError); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// EnabledSources 启用的来源
func (d *DB) EnabledSources() ([]Source, error) {
	rows, err := d.sql.Query(`SELECT id, name, url, category, enabled, last_fetch_at, last_status, last_error FROM sources WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Source{}
	for rows.Next() {
		var s Source
		rows.Scan(&s.ID, &s.Name, &s.URL, &s.Category, &s.Enabled, &s.LastFetch, &s.LastStatus, &s.LastError)
		list = append(list, s)
	}
	return list, nil
}

// AddSource 新增来源
func (d *DB) AddSource(name, url, category string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`INSERT OR IGNORE INTO sources (name, url, category) VALUES (?, ?, ?)`, name, url, nullIfEmpty(category))
	return err
}

// ToggleSource 启停来源
func (d *DB) ToggleSource(id int64) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`UPDATE sources SET enabled = 1 - enabled WHERE id = ?`, id)
	return err
}

// DeleteSource 删除来源
func (d *DB) DeleteSource(id int64) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`DELETE FROM sources WHERE id = ?`, id)
	return err
}

// UpdateSourceHealth 记录来源抓取状态
func (d *DB) UpdateSourceHealth(id int64, status string, errMsg string) {
	mu.Lock()
	defer mu.Unlock()
	d.sql.Exec(`UPDATE sources SET last_fetch_at=?, last_status=?, last_error=? WHERE id=?`,
		time.Now().Format(time.RFC3339), status, nullIfEmpty(errMsg), id)
}

// SeedDefaults 首次运行写入默认安全源（40+ 国内外信息源）
func (d *DB) SeedDefaults() {
	var n int
	d.sql.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&n)
	if n > 0 {
		return
	}
	defaults := []Source{
		// === 国内安全媒体（2026-10 逐源实测可用）===
		{Name: "FreeBuf", URL: "https://www.freebuf.com/feed", Category: strPtr("安全媒体")},
		{Name: "安全客", URL: "https://api.anquanke.com/data/v1/rss", Category: strPtr("安全媒体")},
		{Name: "嘶吼", URL: "https://www.4hou.com/feed", Category: strPtr("安全媒体")},
		{Name: "安全脉搏", URL: "https://www.secpulse.com/feed", Category: strPtr("安全媒体")},

		// === 技术研究 ===
		{Name: "先知社区", URL: "https://xz.aliyun.com/feed", Category: strPtr("技术研究")},
		{Name: "Seebug Paper", URL: "https://paper.seebug.org/rss/", Category: strPtr("技术研究")},
		{Name: "腾讯玄武实验室", URL: "https://xlab.tencent.com/cn/atom.xml", Category: strPtr("技术研究")},
		{Name: "Google Project Zero", URL: "https://googleprojectzero.blogspot.com/feeds/posts/default", Category: strPtr("技术研究")},
		{Name: "KitPloit", URL: "https://www.kitploit.com/feeds/posts/default", Category: strPtr("技术研究")},

		// === 厂商研究 ===
		{Name: "绿盟科技", URL: "https://blog.nsfocus.net/feed/", Category: strPtr("厂商研究")},
		{Name: "微软安全", URL: "https://www.microsoft.com/security/blog/feed/", Category: strPtr("厂商研究")},
		{Name: "Palo Alto Unit42", URL: "https://unit42.paloaltonetworks.com/feed/", Category: strPtr("厂商研究")},
		{Name: "Cisco Talos", URL: "https://blog.talosintelligence.com/rss/", Category: strPtr("厂商研究")},
		{Name: "卡巴斯基 Securelist", URL: "https://securelist.com/feed/", Category: strPtr("厂商研究")},
		{Name: "Check Point", URL: "https://blog.checkpoint.com/feed/", Category: strPtr("厂商研究")},
		{Name: "ESET WeLiveSecurity", URL: "https://www.welivesecurity.com/feed/", Category: strPtr("厂商研究")},
		{Name: "Malwarebytes", URL: "https://www.malwarebytes.com/blog/feed", Category: strPtr("厂商研究")},

		// === 漏洞通报 ===
		{Name: "CISA 新闻", URL: "https://www.cisa.gov/news.xml", Category: strPtr("漏洞通报")},
		{Name: "CISA 安全通告", URL: "https://www.cisa.gov/cybersecurity-advisories/all.xml", Category: strPtr("漏洞通报")},
		{Name: "英国 NCSC", URL: "https://www.ncsc.gov.uk/api/1/services/v1/report-rss-feed.xml", Category: strPtr("漏洞通报")},

		// === 国际媒体 ===
		{Name: "The Hacker News", URL: "https://feeds.feedburner.com/TheHackersNews", Category: strPtr("国际媒体")},
		{Name: "BleepingComputer", URL: "https://www.bleepingcomputer.com/feed/", Category: strPtr("国际媒体")},
		{Name: "SecurityWeek", URL: "https://www.securityweek.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Dark Reading", URL: "https://www.darkreading.com/rss.xml", Category: strPtr("国际媒体")},
		{Name: "SANS ISC", URL: "https://isc.sans.edu/rssfeed_full.xml", Category: strPtr("国际媒体")},
		{Name: "Krebs on Security", URL: "https://krebsonsecurity.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Schneier on Security", URL: "https://www.schneier.com/blog/atom.xml", Category: strPtr("国际媒体")},
		{Name: "Ars Technica Security", URL: "https://arstechnica.com/security/feed/", Category: strPtr("国际媒体")},
		{Name: "The Record", URL: "https://therecord.media/feed", Category: strPtr("国际媒体")},
		{Name: "Security Affairs", URL: "https://securityaffairs.com/feed/", Category: strPtr("国际媒体")},
		{Name: "The DFIR Report", URL: "https://thedfirreport.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Help Net Security", URL: "https://www.helpnetsecurity.com/feed/", Category: strPtr("国际媒体")},
		{Name: "CSO Online", URL: "https://www.csoonline.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Graham Cluley", URL: "https://grahamcluley.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Wired Security", URL: "https://www.wired.com/feed/rss", Category: strPtr("国际媒体")},
		{Name: "Troy Hunt", URL: "https://www.troyhunt.com/rss/", Category: strPtr("国际媒体")},
		{Name: "TechCrunch Security", URL: "https://techcrunch.com/category/security/feed/", Category: strPtr("国际媒体")},
		{Name: "TorrentFreak", URL: "https://torrentfreak.com/feed/", Category: strPtr("国际媒体")},
		{Name: "Infosecurity Magazine", URL: "https://www.infosecurity-magazine.com/rss/news/", Category: strPtr("国际媒体")},

		// === AI 安全 ===
		{Name: "量子位", URL: "https://www.qbitai.com/feed", Category: strPtr("AI 安全")},
	}

	for _, s := range defaults {
		d.AddSource(s.Name, s.URL, deref(s.Category))
	}
}

// ---------- Settings ----------

// GetSetting 读设置
func (d *DB) GetSetting(key, def string) string {
	var v sql.NullString
	d.sql.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if !v.Valid || v.String == "" {
		return def
	}
	return v.String
}

// SetSetting 写设置
func (d *DB) SetSetting(key, value string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// AllSettings 全部设置（脱敏 key 不在此返回）
func (d *DB) AllSettings() map[string]string {
	rows, err := d.sql.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return map[string]string{}
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		m[k] = v
	}
	return m
}

// ---------- Briefing ----------

// SaveBriefing 保存简报
func (d *DB) SaveBriefing(date, content string) error {
	mu.Lock()
	defer mu.Unlock()
	_, err := d.sql.Exec(`INSERT INTO briefings (date, content, generated_at) VALUES (?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET content=excluded.content, generated_at=excluded.generated_at`,
		date, content, time.Now().Format(time.RFC3339))
	return err
}

// GetBriefing 按日期取简报
func (d *DB) GetBriefing(date string) (string, error) {
	var c sql.NullString
	err := d.sql.QueryRow(`SELECT content FROM briefings WHERE date=?`, date).Scan(&c)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return c.String, err
}

// ---------- helpers ----------

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func toJSONArr(arr []string) any {
	if len(arr) == 0 {
		return nil
	}
	b, _ := json.Marshal(arr)
	return string(b)
}

func strPtr(s string) *string { return &s }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var _ = fmt.Sprintf // 保持 fmt 引用（调试用）
