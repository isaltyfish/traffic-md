package tracker

import (
	"fmt"
	"log"
	"time"

	"traffic-md/internal/iputil"
	"traffic-md/internal/parser"
	"traffic-md/internal/stats"

	"github.com/gaols/tail"
)

// Tracker 跟踪nginx日志并统计流量
type Tracker struct {
	logPath      string
	statsManager *stats.StatsManager
	logParser    *parser.LogParser
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewTracker 创建新的跟踪器
func NewTracker(logPath string, endpointRules []string) *Tracker {
	return &Tracker{
		logPath:      logPath,
		statsManager: stats.NewStatsManager(endpointRules),
		logParser:    parser.NewLogParser(),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start 开始跟踪日志
func (t *Tracker) Start() error {
	// 启动tail
	lineCh, closeFunc, errCh := tail.TailF(t.logPath, true)

	// 启动每日重置检查
	go t.dailyResetLoop()

	// 启动淘汰检查
	go t.evictLoop()

	// 启动QPS更新循环（每秒更新一次）
	go t.qpsUpdateLoop()

	// 处理日志行
	go func() {
		defer close(t.doneCh)
		for {
			select {
			case <-t.stopCh:
				closeFunc()
				return
			case err := <-errCh:
				if err != nil {
					log.Printf("Error tailing log: %v", err)
				}
			case line := <-lineCh:
				// 处理所有行，包括空行（parser 内部会忽略空行）
				t.processLogLine(line)
			}
		}
	}()

	return nil
}

// Stop 停止跟踪
func (t *Tracker) Stop() {
	close(t.stopCh)
	<-t.doneCh
}

// processLogLine 处理单行日志
// 如果解析失败或返回空，会静默忽略，不会影响整体统计
func (t *Tracker) processLogLine(line string) {
	entry, err := t.logParser.Parse(line)
	// 忽略解析失败的行（空行、格式不匹配等）
	// parser 设计为不会返回错误，只会返回 nil, nil
	if err != nil || entry == nil {
		return
	}

	// 更新IP统计
	ipStat := t.statsManager.GetIPStats().GetOrCreate(entry.IP)
	ipStat.AddBytes(entry.BytesSent)

	// 更新按IP类型的日统计
	ipType := iputil.ClassifyIP(entry.IP)
	dailyStat := t.statsManager.GetCurrentDay()

	switch ipType {
	case iputil.IPTypePublic:
		dailyStat.AddPublicBytes(entry.BytesSent)
	case iputil.IPTypePrivate, iputil.IPTypeLoopback:
		dailyStat.AddPrivateBytes(entry.BytesSent)
	default:
		dailyStat.AddOtherBytes(entry.BytesSent)
	}

	// 更新端点统计
	if entry.Path != "" {
		if endpointPattern := t.statsManager.MatchEndpoint(entry.Path); endpointPattern != "" {
			dailyStat.AddEndpointBytes(endpointPattern, entry.BytesSent)
			// 更新端点QPS
			dailyStat.AddEndpointRequest(endpointPattern)
		}
	}

	// 更新总QPS（每个请求都计数）
	dailyStat.AddRequest()
}

// dailyResetLoop 每日重置循环
func (t *Tracker) dailyResetLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			if t.statsManager.CheckAndResetDay() {
				log.Println("New day started, statistics reset")
			}
		}
	}
}

// evictLoop 淘汰检查循环
func (t *Tracker) evictLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			before := t.statsManager.GetIPStats().Count()
			t.statsManager.GetIPStats().EvictIfNeeded()
			after := t.statsManager.GetIPStats().Count()
			if before != after {
				log.Printf("Evicted IPs: %d -> %d", before, after)
			}
		}
	}
}

// qpsUpdateLoop QPS更新循环（每秒更新一次）
func (t *Tracker) qpsUpdateLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.statsManager.UpdateQPS()
		}
	}
}

// GetStatsManager 获取统计管理器（用于 API）
func (t *Tracker) GetStatsManager() *stats.StatsManager {
	return t.statsManager
}

// GetStats 获取统计信息（用于展示）
func (t *Tracker) GetStats() *StatsReport {
	daily := t.statsManager.GetCurrentDay()
	ipStats := t.statsManager.GetIPStats()

	return &StatsReport{
		Date:          daily.Date,
		PublicBytes:   daily.GetPublicBytes(),
		PrivateBytes:  daily.GetPrivateBytes(),
		OtherBytes:    daily.GetOtherBytes(),
		EndpointStats: daily.GetEndpointStats(),
		IPCount:       ipStats.Count(),
	}
}

// StatsReport 统计报告
type StatsReport struct {
	Date          time.Time
	PublicBytes   int64
	PrivateBytes  int64
	OtherBytes    int64
	EndpointStats map[string]int64
	IPCount       int
}

// Print 打印统计报告
func (r *StatsReport) Print() {
	fmt.Printf("\n=== Traffic Statistics Report ===\n")
	fmt.Printf("Date: %s\n", r.Date.Format("2006-01-02"))
	fmt.Printf("Public IP Traffic: %.2f GB\n", float64(r.PublicBytes)/(1024*1024*1024))
	fmt.Printf("Private IP Traffic: %.2f GB\n", float64(r.PrivateBytes)/(1024*1024*1024))
	fmt.Printf("Other Traffic: %.2f GB\n", float64(r.OtherBytes)/(1024*1024*1024))
	fmt.Printf("Total IPs Tracked: %d\n", r.IPCount)
	fmt.Printf("\nEndpoint Statistics:\n")
	for endpoint, bytes := range r.EndpointStats {
		fmt.Printf("  %s: %.2f GB\n", endpoint, float64(bytes)/(1024*1024*1024))
	}
	fmt.Printf("================================\n\n")
}
