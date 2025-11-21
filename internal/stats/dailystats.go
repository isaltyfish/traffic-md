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
	PublicBytes   int64             // 公网流量
	PrivateBytes  int64             // 内网流量
	OtherBytes    int64             // 其他流量
	EndpointStats map[string]*int64 // 端点流量统计（预先创建，使用 atomic 操作计数器，无需锁）
}

// NewDailyStats 创建新的日统计
// endpointRules: 端点规则列表，用于预先创建所有端点的计数器
func NewDailyStats(date time.Time, endpointRules []string) *DailyStats {
	// 预先创建所有端点的计数器
	endpointStats := make(map[string]*int64, len(endpointRules))
	for _, rule := range endpointRules {
		counter := new(int64) // 初始值为 0
		endpointStats[rule] = counter
	}

	return &DailyStats{
		Date:          date,
		EndpointStats: endpointStats,
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

func (d *DailyStats) AddEndpointBytes(endpoint string, bytes int64) {
	if counter, exists := d.EndpointStats[endpoint]; exists {
		atomic.AddInt64(counter, bytes)
	}
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
	result := make(map[string]int64, len(d.EndpointStats))
	// 由于 map 结构固定（只读），遍历是安全的
	// 使用 atomic 读取每个计数器的值
	for endpoint, counter := range d.EndpointStats {
		result[endpoint] = atomic.LoadInt64(counter)
	}
	return result
}

// StatsManager 管理所有统计
type StatsManager struct {
	currentDay      *DailyStats
	ipStats         *IPStatsManager
	endpointRules   []string            // 需要统计的端点规则，如 "/abc/def/**"
	patternSegments map[string][]string // 预处理的 pattern segments，key: pattern, value: segments
	mu              sync.RWMutex
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(endpointRules []string) *StatsManager {
	now := time.Now()

	// 预处理所有 pattern segments，避免每次匹配时重复 split
	patternSegments := make(map[string][]string, len(endpointRules))
	for _, rule := range endpointRules {
		// 规范化 pattern
		normalizedPattern := rule
		if !strings.HasPrefix(normalizedPattern, "/") {
			normalizedPattern = "/" + normalizedPattern
		}
		patternSegments[rule] = splitPath(normalizedPattern)
	}

	return &StatsManager{
		currentDay:      NewDailyStats(now, endpointRules),
		ipStats:         NewIPStatsManager(),
		endpointRules:   endpointRules,
		patternSegments: patternSegments,
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
		m.currentDay = NewDailyStats(now, m.endpointRules)
		m.ipStats.Reset()
		return true
	}
	return false
}

// MatchEndpoint 检查路径是否匹配端点规则，返回匹配的规则pattern
// 如果匹配多个规则，返回第一个匹配的规则
// 如果没有匹配，返回空字符串
func (m *StatsManager) MatchEndpoint(path string) string {
	// 规范化路径：确保以 / 开头
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	pathSegments := splitPath(normalizedPath)

	// 使用预处理的 pattern segments
	for _, rule := range m.endpointRules {
		patternSegments := m.patternSegments[rule]
		if matchSegments(pathSegments, patternSegments) {
			return rule
		}
	}
	return ""
}

// matchPath 实现 glob 模式匹配，支持 * 和 **
// * 匹配单个路径段（不包含 /）
// ** 匹配零个或多个路径段（可以包含 /）
// 注意：此函数保留用于测试，实际使用中应使用 MatchEndpoint（使用预处理的 segments）
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
