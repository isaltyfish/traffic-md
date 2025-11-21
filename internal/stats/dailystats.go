package stats

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DailyStats 按天统计的流量数据
type DailyStats struct {
	Date          time.Time
	PublicBytes   int64            // 公网流量
	PrivateBytes  int64            // 内网流量
	OtherBytes    int64            // 其他流量
	EndpointStats map[string]int64 // 端点流量统计
	mu            sync.RWMutex
}

// NewDailyStats 创建新的日统计
func NewDailyStats(date time.Time) *DailyStats {
	return &DailyStats{
		Date:          date,
		EndpointStats: make(map[string]int64),
	}
}

// AddPublicBytes 增加公网流量（使用原子操作，无锁）
func (d *DailyStats) AddPublicBytes(bytes int64) {
	atomic.AddInt64(&d.PublicBytes, bytes)
}

// AddPrivateBytes 增加内网流量（使用原子操作，无锁）
func (d *DailyStats) AddPrivateBytes(bytes int64) {
	atomic.AddInt64(&d.PrivateBytes, bytes)
}

// AddOtherBytes 增加其他流量（使用原子操作，无锁）
func (d *DailyStats) AddOtherBytes(bytes int64) {
	atomic.AddInt64(&d.OtherBytes, bytes)
}

// AddEndpointBytes 增加端点流量
func (d *DailyStats) AddEndpointBytes(endpoint string, bytes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.EndpointStats[endpoint] += bytes
}

// GetPublicBytes 获取公网流量（使用原子操作，无锁）
func (d *DailyStats) GetPublicBytes() int64 {
	return atomic.LoadInt64(&d.PublicBytes)
}

// GetPrivateBytes 获取内网流量（使用原子操作，无锁）
func (d *DailyStats) GetPrivateBytes() int64 {
	return atomic.LoadInt64(&d.PrivateBytes)
}

// GetOtherBytes 获取其他流量（使用原子操作，无锁）
func (d *DailyStats) GetOtherBytes() int64 {
	return atomic.LoadInt64(&d.OtherBytes)
}

// GetEndpointStats 获取端点统计
func (d *DailyStats) GetEndpointStats() map[string]int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make(map[string]int64)
	for k, v := range d.EndpointStats {
		result[k] = v
	}
	return result
}

// StatsManager 管理所有统计
type StatsManager struct {
	currentDay    *DailyStats
	ipStats       *IPStatsManager
	endpointRules []string // 需要统计的端点规则，如 "/abc/def/**"
	mu            sync.RWMutex
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(endpointRules []string) *StatsManager {
	now := time.Now()
	return &StatsManager{
		currentDay:    NewDailyStats(now),
		ipStats:       NewIPStatsManager(),
		endpointRules: endpointRules,
	}
}

// GetCurrentDay 获取当前天的统计
func (m *StatsManager) GetCurrentDay() *DailyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentDay
}

// GetIPStats 获取IP统计管理器
func (m *StatsManager) GetIPStats() *IPStatsManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ipStats
}

// CheckAndResetDay 检查是否需要重置到新的一天
func (m *StatsManager) CheckAndResetDay() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	currentDate := time.Date(m.currentDay.Date.Year(), m.currentDay.Date.Month(), m.currentDay.Date.Day(), 0, 0, 0, 0, m.currentDay.Date.Location())
	newDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if !currentDate.Equal(newDate) {
		// 新的一天，重置统计
		m.currentDay = NewDailyStats(now)
		m.ipStats.Reset()
		return true
	}
	return false
}

// MatchEndpoint 检查路径是否匹配端点规则，返回匹配的规则pattern
// 如果匹配多个规则，返回第一个匹配的规则
// 如果没有匹配，返回空字符串
func (m *StatsManager) MatchEndpoint(path string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rule := range m.endpointRules {
		if matchPath(path, rule) {
			return rule
		}
	}
	return ""
}

// matchPath 实现 glob 模式匹配，支持 * 和 **
// * 匹配单个路径段（不包含 /）
// ** 匹配零个或多个路径段（可以包含 /）
func matchPath(path, pattern string) bool {
	// 规范化路径：确保以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}

	// 分割路径和模式为段
	pathSegments := splitPath(path)
	patternSegments := splitPath(pattern)

	return matchSegments(pathSegments, patternSegments)
}

// splitPath 将路径分割为段，忽略空段
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	segments := strings.Split(path, "/")
	// 过滤空段
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}
	return result
}

// matchSegments 递归匹配路径段和模式段
func matchSegments(pathSegs, patternSegs []string) bool {
	// 如果模式用完了，路径也必须用完
	if len(patternSegs) == 0 {
		return len(pathSegs) == 0
	}

	// 如果路径用完了，检查剩余的模式是否都是 **
	if len(pathSegs) == 0 {
		for _, p := range patternSegs {
			if p != "**" {
				return false
			}
		}
		return true
	}

	pattern := patternSegs[0]

	// 处理 ** 通配符（匹配零个或多个路径段）
	if pattern == "**" {
		// ** 可以匹配零个或多个段
		// 尝试匹配零个段
		if matchSegments(pathSegs, patternSegs[1:]) {
			return true
		}
		// 尝试匹配一个或多个段
		for i := 1; i <= len(pathSegs); i++ {
			if matchSegments(pathSegs[i:], patternSegs[1:]) {
				return true
			}
		}
		return false
	}

	// 处理 * 通配符（匹配单个路径段）
	if pattern == "*" {
		return matchSegments(pathSegs[1:], patternSegs[1:])
	}

	// 精确匹配
	if pattern == pathSegs[0] {
		return matchSegments(pathSegs[1:], patternSegs[1:])
	}

	return false
}
