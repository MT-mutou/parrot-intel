package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// sqlExec 在独立连接上执行 DDL（测试中构造 v2.2.x 旧表结构用）
func sqlExec(path, ddl string) (sql.Result, error) {
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.Exec(ddl)
}

// 回归测试：无正文（content 为 NULL）的文章，详情查询必须正常返回而不是报错。
// v2.2.0 及之前版本将 content/collected_at 直接扫进 string，遇到 NULL 报
// "converting NULL to string is unsupported"，被上层误判为「文章不存在」返回 404。
func TestGetArticleNullContent(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.sql.Close()

	// summary / content / publishedAt 均为空 → 入库为 NULL
	if !d.InsertArticle("nullcontent001", "测试文章-无正文", "https://example.com/a/1",
		"测试源", "https://example.com", "", "", "") {
		t.Fatal("插入文章失败")
	}

	a, err := d.GetArticle("nullcontent001")
	if err != nil {
		t.Fatalf("GetArticle 对无正文文章报错（v2.2.0 的 404 根因）: %v", err)
	}
	if a == nil || a.Title != "测试文章-无正文" {
		t.Fatalf("返回数据异常: %+v", a)
	}
	if a.Content != "" {
		t.Fatalf("content 应为空字符串, 实际 %q", a.Content)
	}

	// Unanalyzed 同样不应因 NULL content 报错
	list, err := d.Unanalyzed(10)
	if err != nil {
		t.Fatalf("Unanalyzed 报错: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("Unanalyzed 应返回 1 篇, 实际 %d", len(list))
	}
}

// 回归测试：SeedDefaults 对已存在的同名源应更新 URL（旧库升级修复失效地址），
// 且不重复插入、不清空启停状态。
func TestSeedDefaultsSyncsExistingURL(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.sql.Close()

	// 模拟 v2.1.0 旧库：安全客为已失效的旧地址
	if err := d.AddSource("安全客", "https://www.anquanke.com/rss", "安全媒体", "", ""); err != nil {
		t.Fatalf("预置旧源失败: %v", err)
	}
	d.SeedDefaults()

	var url string
	if err := d.sql.QueryRow(`SELECT url FROM sources WHERE name='安全客'`).Scan(&url); err != nil {
		t.Fatalf("查询安全客失败: %v", err)
	}
	if url != "https://api.anquanke.com/data/v1/rss" {
		t.Fatalf("旧地址未同步为新的 API 地址, 实际: %s", url)
	}

	// 再跑一次不应产生重复
	d.SeedDefaults()
	var n int
	d.sql.QueryRow(`SELECT COUNT(*) FROM sources WHERE name='安全客'`).Scan(&n)
	if n != 1 {
		t.Fatalf("SeedDefaults 重复插入了安全客 %d 条", n)
	}
}

// 回归测试：v2.2.2 爬虫收集器 —— sources 表 type/config 列迁移、
// 默认源带类型写入、SetArticleMeta 不影响 ai_analyzed。
func TestCrawlerSourceAndMeta(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.sql.Close()

	d.SeedDefaults()

	// NVD / GitHub 默认源应带类型
	var typ string
	if err := d.sql.QueryRow(`SELECT type FROM sources WHERE name='NVD 最新漏洞'`).Scan(&typ); err != nil {
		t.Fatalf("查询 NVD 源失败: %v", err)
	}
	if typ != "nvd" {
		t.Fatalf("NVD 源 type 应为 nvd, 实际 %q", typ)
	}
	if err := d.sql.QueryRow(`SELECT type FROM sources WHERE name='GitHub 安全通告'`).Scan(&typ); err != nil {
		t.Fatalf("查询 GitHub 源失败: %v", err)
	}
	if typ != "ghsa" {
		t.Fatalf("GitHub 源 type 应为 ghsa, 实际 %q", typ)
	}
	if err := d.sql.QueryRow(`SELECT type FROM sources WHERE name='FreeBuf'`).Scan(&typ); err != nil {
		t.Fatalf("查询 FreeBuf 失败: %v", err)
	}
	if typ != "rss" {
		t.Fatalf("FreeBuf 源 type 应为 rss, 实际 %q", typ)
	}

	// 旧库迁移路径：手工删掉 type 列语义由 ALTER 补齐，此处验证 EnabledSources 对旧类型空值兜底为 rss
	if err := d.AddSource("测试爬虫源", "https://example.com/list", "厂商动态", "html",
		`{"item":".news li","title":"a","link":"a"}`); err != nil {
		t.Fatalf("添加 html 源失败: %v", err)
	}
	srcs, err := d.EnabledSources()
	if err != nil {
		t.Fatalf("EnabledSources: %v", err)
	}
	found := false
	for _, s := range srcs {
		if s.Name == "测试爬虫源" {
			found = true
			if s.Type != "html" || s.Config == nil || *s.Config == "" {
				t.Fatalf("html 源 type/config 未正确保存: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("EnabledSources 未返回测试爬虫源")
	}

	// SetArticleMeta：写威胁等级/CVE，不置 ai_analyzed
	if !d.InsertArticle("metatest001", "CVE-2026-1234 测试", "https://nvd.nist.gov/vuln/detail/CVE-2026-1234",
		"NVD 最新漏洞", "https://services.nvd.nist.gov", "summary", "content", "") {
		t.Fatal("插入文章失败")
	}
	if err := d.SetArticleMeta("metatest001", "high", []string{"CVE-2026-1234"}, []string{"NVD"}); err != nil {
		t.Fatalf("SetArticleMeta: %v", err)
	}
	a, err := d.GetArticle("metatest001")
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if a.ThreatLevel == nil || *a.ThreatLevel != "high" || len(a.CVEIDs) != 1 || a.CVEIDs[0] != "CVE-2026-1234" {
		t.Fatalf("SetArticleMeta 未生效: threat=%v cves=%v", a.ThreatLevel, a.CVEIDs)
	}
	if a.AIAnalyzed != 0 {
		t.Fatal("SetArticleMeta 不应置 ai_analyzed=1（AI 需可继续分析）")
	}
}

// v2.3.0 角色化简报：briefing_profiles 种子模板、briefings 复合主键迁移、
// 同一日期多模板简报互不覆盖、旧表单主键数据迁移到 profile_id=0。
func TestBriefingProfiles(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.sql.Close()

	// 种子：4 个内置角色模板
	profiles, err := d.ListBriefingProfiles()
	if err != nil {
		t.Fatalf("ListBriefingProfiles: %v", err)
	}
	if len(profiles) != 4 {
		t.Fatalf("应种子 4 个内置模板, 实际 %d", len(profiles))
	}
	first, err := d.GetBriefingProfile(profiles[0].ID)
	if err != nil || first.Name != "领导决策版" || first.Role != "executive" || len(first.Sections) != 4 {
		t.Fatalf("领导决策版模板异常: %+v err=%v", first, err)
	}

	// 同一日期多模板简报：互不覆盖
	if err := d.SaveBriefing("2026-10-29", profiles[0].ID, "领导版内容"); err != nil {
		t.Fatalf("SaveBriefing: %v", err)
	}
	if err := d.SaveBriefing("2026-10-29", profiles[1].ID, "安全管理版内容"); err != nil {
		t.Fatalf("SaveBriefing: %v", err)
	}
	// 同模板同日期重新生成 → 覆盖
	if err := d.SaveBriefing("2026-10-29", profiles[0].ID, "领导版内容v2"); err != nil {
		t.Fatalf("SaveBriefing: %v", err)
	}
	c0, _ := d.GetBriefing("2026-10-29", profiles[0].ID)
	c1, _ := d.GetBriefing("2026-10-29", profiles[1].ID)
	if c0 != "领导版内容v2" || c1 != "安全管理版内容" {
		t.Fatalf("多模板简报隔离失败: c0=%q c1=%q", c0, c1)
	}

	// 历史列表按模板过滤
	list0, _ := d.ListBriefingDates(profiles[0].ID, 30)
	list9, _ := d.ListBriefingDates(9999, 30)
	if len(list0) != 1 || len(list9) != 0 {
		t.Fatalf("ListBriefingDates 过滤异常: p0=%d p9999=%d", len(list0), len(list9))
	}

	// 删除模板连带删除其简报
	if err := d.DeleteBriefingProfile(profiles[0].ID); err != nil {
		t.Fatalf("DeleteBriefingProfile: %v", err)
	}
	c0, _ = d.GetBriefing("2026-10-29", profiles[0].ID)
	if c0 != "" {
		t.Fatalf("删除模板后其简报应一并删除, 实际 %q", c0)
	}

	// 模板 CRUD
	id, err := d.AddBriefingProfile(&BriefingProfile{
		Name: "周报·金融风控", Role: "custom", Sections: []string{"policy", "incidents"},
		Detail: "brief", CustomPrompt: "面向集团风控委员会", Enabled: 1,
	})
	if err != nil {
		t.Fatalf("AddBriefingProfile: %v", err)
	}
	got, _ := d.GetBriefingProfile(id)
	if got.CustomPrompt != "面向集团风控委员会" || len(got.Sections) != 2 {
		t.Fatalf("自定义模板读取异常: %+v", got)
	}
	got.Detail = "detailed"
	if err := d.UpdateBriefingProfile(got); err != nil {
		t.Fatalf("UpdateBriefingProfile: %v", err)
	}
	got2, _ := d.GetBriefingProfile(id)
	if got2.Detail != "detailed" {
		t.Fatalf("更新未生效: %+v", got2)
	}
}

// v2.3.0 旧库迁移：v2.2.x 的 briefings 表以 date 为单主键，
// 升级后应保留旧简报（profile_id=0）且支持新复合主键写入。
func TestBriefingsTableMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parrot.db")

	// 先用 v2.2.x 的旧表结构建库
	old, err := Open(dir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	old.sql.Close()
	// 手工降级：删掉新表，按旧结构重建并写入一条旧简报
	if _, err := sqlExec(path, `DROP TABLE briefings; CREATE TABLE briefings (date TEXT PRIMARY KEY, content TEXT, generated_at TEXT); INSERT INTO briefings VALUES ('2026-10-01', '旧版通用简报', '2026-10-01T09:00:00+08:00');`); err != nil {
		t.Fatalf("构造旧表失败: %v", err)
	}

	// 重新 Open → 触发 v2.3.0 迁移
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("重新打开触发迁移失败: %v", err)
	}
	defer d.sql.Close()

	// 旧简报迁移到 profile_id=0
	c, err := d.GetBriefing("2026-10-01", 0)
	if err != nil || c != "旧版通用简报" {
		t.Fatalf("旧简报未迁移: c=%q err=%v", c, err)
	}
	// 新复合主键：同日期不同模板可并存
	if err := d.SaveBriefing("2026-10-01", 7, "新模板简报"); err != nil {
		t.Fatalf("迁移后写入新模板简报失败: %v", err)
	}
	c7, _ := d.GetBriefing("2026-10-01", 7)
	if c7 != "新模板简报" {
		t.Fatalf("新模板简报读取异常: %q", c7)
	}
}
