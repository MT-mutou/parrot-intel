// 鹦鹉情报站 ParrotIntel —— 威胁情报聚合平台 API 服务（后端）
//
// 前端为独立 Vue3 工程（../web），由 Nginx 部署并通过 /api 反代访问本服务。
//
// 用法：parrot-intel-server [-addr :8080] [-data ./data] [-cors *]
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/auth"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
	"parrot-intel/internal/notify"
	"parrot-intel/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dataDir := flag.String("data", "data", "数据目录（SQLite）")
	cors := flag.String("cors", "*", "允许的前端来源（CORS），逗号分隔，* 表示全部")
	flag.Parse()

	lg := log.New(os.Stdout, "", log.LstdFlags)

	database, err := db.Open(*dataDir)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	database.SeedDefaults()

	// 登录账号初始化：默认 yingwu / yingwu，首次登录强制修改密码
	am := auth.NewManager()
	if database.GetSetting("admin_user", "") == "" {
		database.SetSetting("admin_user", "yingwu")
	}
	if database.GetSetting("admin_pass_hash", "") == "" {
		if h, err := auth.HashPassword("yingwu"); err == nil {
			database.SetSetting("admin_pass_hash", h)
			database.SetSetting("force_change_pwd", "true")
		}
	}
	if database.GetSetting("force_change_pwd", "") == "" {
		database.SetSetting("force_change_pwd", "true")
	}

	// AI 客户端工厂：每次调用实时读取设置（改配置无需重启）
	aiFn := func() *ai.Client {
		all := database.AllSettings()
		return ai.New(ai.Config{
			Endpoint: orDef(all["ai_endpoint"], "https://api.deepseek.com/v1"),
			APIKey:   all["ai_api_key"],
			Model:    orDef(all["ai_model"], "deepseek-chat"),
		})
	}

	col := &collector.Collector{
		DB:  database,
		AI:  aiFn,
		Log: lg,
	}

	// 收集完成后：若开启自动简报推送，生成并推送到所有启用渠道
	col.OnDone = func(inserted, analyzed int) {
		if database.GetSetting("auto_briefing_push", "") != "true" {
			return
		}
		go autoBriefing(database, aiFn, lg)
	}

	srv := web.New(database, col, aiFn, lg, *cors, am)

	// 定时收集：每分钟检查是否到达设定小时
	go func() {
		lastRun := ""
		for {
			time.Sleep(time.Minute)
			hourStr := database.GetSetting("collect_hour", "9")
			hour, err := strconv.Atoi(hourStr)
			if err != nil || hour < 0 || hour > 23 {
				hour = 9
			}
			today := time.Now().Format("2006-01-02")
			nowHour := time.Now().Hour()
			if nowHour == hour && lastRun != today {
				lastRun = today
				lg.Printf("[cron] 触发每日收集 (%02d:00)", hour)
				col.RunNow()
			}
		}
	}()

	lg.Printf("鹦鹉情报站 API 服务启动: http://0.0.0.0%s （数据目录 %s，CORS %s）", *addr, *dataDir, *cors)
	server := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

// autoBriefing 生成当日简报并推送全部启用渠道
func autoBriefing(database *db.DB, aiFn func() *ai.Client, lg *log.Logger) {
	items, err := database.TodayArticles()
	if err != nil || len(items) == 0 {
		lg.Printf("[auto-briefing] 今日无文章，跳过")
		return
	}
	c := aiFn()
	if !c.Configured() {
		lg.Printf("[auto-briefing] 未配置 AI，跳过")
		return
	}
	var bis []ai.BriefingItem
	for _, it := range items {
		sum, threat := "", ""
		if it.Summary != nil {
			sum = *it.Summary
		}
		if it.ThreatLevel != nil {
			threat = *it.ThreatLevel
		}
		bis = append(bis, ai.BriefingItem{Title: it.Title, Summary: sum, Threat: threat})
	}
	md, err := c.GenerateBriefing(bis)
	if err != nil {
		lg.Printf("[auto-briefing] 生成失败: %v", err)
		return
	}
	date := time.Now().Format("2006-01-02")
	database.SaveBriefing(date, md)

	channels, _ := database.EnabledChannels()
	if len(channels) == 0 {
		lg.Printf("[auto-briefing] 无启用渠道，仅保存")
		return
	}
	title := "🦜 鹦鹉情报站 · 每日安全简报 " + date
	for _, ch := range channels {
		sender, err := notify.NewSender(ch.Type, ch.Webhook, ch.Secret)
		if err != nil {
			database.UpdateChannelStatus(ch.ID, "error: "+err.Error())
			continue
		}
		if err := sender.SendMarkdown(title, notify.TruncateMD(md, 18000)); err != nil {
			database.UpdateChannelStatus(ch.ID, "error: "+err.Error())
			lg.Printf("[auto-briefing] 推送 %s 失败: %v", ch.Name, err)
			continue
		}
		database.UpdateChannelStatus(ch.ID, "success")
		lg.Printf("[auto-briefing] 已推送 %s", ch.Name)
	}
}

func orDef(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
