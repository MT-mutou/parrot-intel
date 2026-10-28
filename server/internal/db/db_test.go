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
	if err := d.AddSource("安全客", "https://www.anquanke.com/rss", "安全媒体"); err != nil {
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
