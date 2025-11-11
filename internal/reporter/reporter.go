package reporter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"traffic-md/internal/tracker"
)

// Reporter 统计报告记录器
type Reporter struct {
	logFile    *os.File
	logPath    string
	mu         sync.Mutex
	firstWrite bool
}

// NewReporter 创建新的报告记录器
func NewReporter() (*Reporter, error) {
	// 获取当前 exe 所在目录
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	logPath := filepath.Join(exeDir, "stat.log")

	// 打开或创建日志文件（追加模式）
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open stat.log: %w", err)
	}

	return &Reporter{
		logFile:    file,
		logPath:    logPath,
		firstWrite: true,
	}, nil
}

// LogStats 记录统计信息到文件
func (r *Reporter) LogStats(stats *tracker.StatsReport) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 如果是首次写入，添加分隔符
	if r.firstWrite {
		r.firstWrite = false
		separator := fmt.Sprintf("\n%s\n", strings.Repeat("=", 80))
		if _, err := r.logFile.WriteString(separator); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	} else {
		// 非首次写入，在报告之间添加空行分隔
		separator := fmt.Sprintf("\n%s\n\n", strings.Repeat("-", 80))
		if _, err := r.logFile.WriteString(separator); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	}

	// 格式化统计信息
	now := time.Now()
	content := r.formatStats(stats, now)

	// 写入文件
	if _, err := r.logFile.WriteString(content); err != nil {
		return fmt.Errorf("failed to write stats: %w", err)
	}

	// 确保数据写入磁盘
	if err := r.logFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// formatStats 格式化统计信息
func (r *Reporter) formatStats(stats *tracker.StatsReport, timestamp time.Time) string {
	var buf strings.Builder

	// 时间戳和日期
	buf.WriteString(fmt.Sprintf("[%s] Traffic Statistics Report\n", timestamp.Format("2006-01-02 15:04:05")))
	buf.WriteString(fmt.Sprintf("Date: %s\n", stats.Date.Format("2006-01-02")))

	// 流量统计
	buf.WriteString(fmt.Sprintf("Public IP Traffic:   %.2f GB (%.2f MB)\n",
		float64(stats.PublicBytes)/(1024*1024*1024),
		float64(stats.PublicBytes)/(1024*1024)))
	buf.WriteString(fmt.Sprintf("Private IP Traffic:  %.2f GB (%.2f MB)\n",
		float64(stats.PrivateBytes)/(1024*1024*1024),
		float64(stats.PrivateBytes)/(1024*1024)))
	buf.WriteString(fmt.Sprintf("Other Traffic:       %.2f GB (%.2f MB)\n",
		float64(stats.OtherBytes)/(1024*1024*1024),
		float64(stats.OtherBytes)/(1024*1024)))

	totalBytes := stats.PublicBytes + stats.PrivateBytes + stats.OtherBytes
	buf.WriteString(fmt.Sprintf("Total Traffic:       %.2f GB (%.2f MB)\n",
		float64(totalBytes)/(1024*1024*1024),
		float64(totalBytes)/(1024*1024)))

	// IP 统计
	buf.WriteString(fmt.Sprintf("Total IPs Tracked:    %d\n", stats.IPCount))

	// 端点统计
	if len(stats.EndpointStats) > 0 {
		buf.WriteString("\nEndpoint Statistics:\n")
		for endpoint, bytes := range stats.EndpointStats {
			buf.WriteString(fmt.Sprintf("  %-40s %.2f GB (%.2f MB)\n",
				endpoint,
				float64(bytes)/(1024*1024*1024),
				float64(bytes)/(1024*1024)))
		}
	}

	// 每个报告末尾添加一个空行
	buf.WriteString("\n")
	return buf.String()
}

// Close 关闭记录器
func (r *Reporter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.logFile != nil {
		return r.logFile.Close()
	}
	return nil
}

// GetLogPath 获取日志文件路径
func (r *Reporter) GetLogPath() string {
	return r.logPath
}
