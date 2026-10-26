// 鹦鹉情报站 ParrotIntel —— 单二进制威胁情报聚合平台
//
// 用法：parrot-intel [-addr :8080] [-data ./data]
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"parrot-intel/internal/ai"
	"parrot-intel/internal/collector"
	"parrot-intel/internal/db"
	"parrot-intel/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dataDir := flag.String("data", "data", "数据目录（SQLite）")
	flag.Parse()

	lg := log.New(os.Stdout, "", log.LstdFlags)

	database, err := db.Open(*dataDir)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	database.SeedDefaults()

	// AI 客户端（设置里配置 key 后生效；收集时实时读取）
	aiCfg := func() ai.Config {
		all := database.AllSettings()
		return ai.Config{
			Endpoint: orDef(all["ai_endpoint"], "https://api.deepseek.com/v1"),
			APIKey:   all["ai_api_key"],
			Model:    orDef(all["ai_model"], "deepseek-chat"),
		}
	}
	col := &collector.Collector{
		DB:  database,
		AI:  ai.New(aiCfg()), // 启动时快照；后续修改设置重启进程生效
		Log: lg,
	}

	srv, err := web.New(database, col, lg)
	if err != nil {
		log.Fatalf("初始化 Web 服务失败: %v", err)
	}

	// 定时收集：每分钟检查一次是否到达设定小时
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

	lg.Printf("鹦鹉情报站启动: http://0.0.0.0%s （数据目录 %s）", *addr, *dataDir)
	server := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}

func orDef(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
