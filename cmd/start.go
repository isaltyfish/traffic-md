/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"traffic-md/internal/api"
	"traffic-md/internal/config"
	"traffic-md/internal/reporter"
	"traffic-md/internal/tracker"

	"github.com/labstack/echo/v4"
	"github.com/spf13/cobra"
)

var (
	logPath        string
	endpointRules  []string
	reportInterval time.Duration
	webPort        int
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start tracking nginx log traffic",
	Long: `Start tracking nginx log traffic and statistics.

This command will:
- Tail nginx log file in real-time
- Track traffic by IP address
- Classify traffic as public/private/other
- Track specific endpoint traffic
- Generate daily statistics

Example:
  traffic-md start --log /var/log/nginx/access.log --endpoints "/api/**" "/static/**"`,
	Run: func(cmd *cobra.Command, args []string) {
		// 加载配置文件
		cfg, err := config.LoadConfig()
		if err != nil {
			log.Printf("Warning: Failed to load config file: %v, using defaults", err)
			// 使用默认配置
			cfg = &config.Config{}
			cfg.Web.Port = 6099
			cfg.Monitor.Endpoints = []string{}
			cfg.Report.Interval = "5m"
		}

		// 应用配置（命令行参数优先级最高）
		// Web 端口
		if !cmd.Flags().Changed("port") {
			webPort = cfg.GetWebPort()
		}

		// 监控端点
		if !cmd.Flags().Changed("endpoints") && len(cfg.GetEndpoints()) > 0 {
			endpointRules = cfg.GetEndpoints()
		}

		// 报告间隔
		if !cmd.Flags().Changed("interval") {
			parsedInterval, err := time.ParseDuration(cfg.GetReportInterval())
			if err != nil {
				log.Printf("Warning: Invalid report.interval in config: %v, using default 5m", err)
				reportInterval = 5 * time.Minute
			} else {
				reportInterval = parsedInterval
			}
		}

		// 日志路径（如果配置文件中设置了，且命令行未指定）
		if logPath == "" {
			if cfg.GetLogPath() != "" {
				logPath = cfg.GetLogPath()
			} else {
				fmt.Fprintf(os.Stderr, "Error: --log flag is required or set log.path in config file\n")
				cmd.Help()
				os.Exit(1)
			}
		}

		// 创建跟踪器
		t := tracker.NewTracker(logPath, endpointRules)

		// 启动跟踪
		if err := t.Start(); err != nil {
			log.Fatalf("Failed to start tracker: %v", err)
		}

		log.Printf("Started tracking log file: %s", logPath)
		if len(endpointRules) > 0 {
			log.Printf("Tracking endpoints: %s", strings.Join(endpointRules, ", "))
		}

		// 创建统计报告记录器
		statsReporter, err := reporter.NewReporter()
		if err != nil {
			log.Printf("Warning: Failed to create stats reporter: %v", err)
			statsReporter = nil
		} else {
			log.Printf("Stats will be logged to: %s", statsReporter.GetLogPath())
		}
		defer func() {
			if statsReporter != nil {
				if err := statsReporter.Close(); err != nil {
					log.Printf("Error closing stats reporter: %v", err)
				}
			}
		}()

		// 启动 Web 服务器
		e := echo.New()
		e.HideBanner = true
		apiServer := api.NewServer(t)
		apiServer.RegisterRoutes(e)

		// 在后台启动 Web 服务器
		webAddr := fmt.Sprintf(":%d", webPort)
		go func() {
			log.Printf("Starting web server on port %d", webPort)
			if err := e.Start(webAddr); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start web server: %v", err)
			}
		}()

		// 设置信号处理
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		// 定期打印统计报告
		reportTicker := time.NewTicker(reportInterval)
		defer reportTicker.Stop()

		// 首次打印
		time.Sleep(2 * time.Second) // 等待一些数据
		stats := t.GetStats()
		stats.Print()
		// 首次记录统计
		if statsReporter != nil {
			if err := statsReporter.LogStats(stats); err != nil {
				log.Printf("Error logging stats: %v", err)
			}
		}

		// 主循环
		for {
			select {
			case <-sigCh:
				log.Println("Received interrupt signal, stopping...")
				// 关闭 Web 服务器
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := e.Shutdown(ctx); err != nil {
					log.Printf("Error shutting down web server: %v", err)
				}
				t.Stop()
				// 打印并记录最终统计
				fmt.Println("\n=== Final Statistics ===")
				finalStats := t.GetStats()
				finalStats.Print()
				if statsReporter != nil {
					if err := statsReporter.LogStats(finalStats); err != nil {
						log.Printf("Error logging final stats: %v", err)
					}
				}
				return
			case <-reportTicker.C:
				stats := t.GetStats()
				stats.Print()
				// 记录统计到文件
				if statsReporter != nil {
					if err := statsReporter.LogStats(stats); err != nil {
						log.Printf("Error logging stats: %v", err)
					}
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&logPath, "log", "l", "", "Path to nginx access log file (required)")
	startCmd.Flags().StringSliceVarP(&endpointRules, "endpoints", "e", []string{}, "Endpoint patterns to track (e.g., '/api/**', '/static/**')")
	startCmd.Flags().DurationVarP(&reportInterval, "interval", "i", 5*time.Minute, "Statistics report interval (e.g., 5m, 1h)")
	startCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "Web server port")
}
