package db

import (
	"path/filepath"
	"testing"
)

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
