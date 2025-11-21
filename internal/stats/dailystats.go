package stats

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SlidingWindow 滑动窗口，用于统计指定时间窗口内的请求数
type SlidingWindow struct {
	requests []time.Time   // 请求时间戳列表
	window   time.Duration // 窗口大小
	mu       sync.RWMutex
}

// NewSlidingWindow 创建新的滑动窗口
func NewSlidingWindow(window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		requests: make([]time.Time, 0, 1000), // 预分配容量
		window:   window,
	}
}

// Add 添加一个请求时间戳
func (sw *SlidingWindow) Add(t time.Time) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.requests = append(sw.requests, t)
	sw.cleanup(t)
}

// Count 获取窗口内的请求数
func (sw *SlidingWindow) Count() int64 {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	// 计算窗口内的请求数
	count := int64(0)
	for _, t := range sw.requests {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

// cleanup 清理过期请求（保留最近窗口大小的请求）
// 使用二分查找优化性能（因为请求是按时间顺序添加的）
func (sw *SlidingWindow) cleanup(now time.Time) {
	cutoff := now.Add(-sw.window)

	if len(sw.requests) == 0 {
		return
	}

	// 如果最早的请求都未过期，不需要清理
	if sw.requests[0].After(cutoff) {
		return
	}

	// 如果最晚的请求都过期，全部清空
	if sw.requests[len(sw.requests)-1].Before(cutoff) {
		sw.requests = sw.requests[:0]
		return
	}

	// 使用二分查找找到第一个未过期的请求索引
	// 由于请求是按时间顺序添加的，可以使用二分查找
	left, right := 0, len(sw.requests)
	for left < right {
		mid := (left + right) / 2
		if sw.requests[mid].After(cutoff) {
			right = mid
		} else {
			left = mid + 1
		}
	}

	// 保留未过期的请求（从left开始）
	if left > 0 {
		sw.requests = sw.requests[left:]
	}
}

// Reset 重置窗口
func (sw *SlidingWindow) Reset() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.requests = sw.requests[:0]
}

// DailyStats 按天统计的流量数据
type DailyStats struct {
	Date          time.Time
	PublicBytes   int64            // 公网流量
	PrivateBytes  int64            // 内网流量
	OtherBytes    int64            // 其他流量
	EndpointStats map[string]int64 // 端点流量统计
	// QPS统计（基于滑动窗口）
	// key: 时间窗口字符串（如 "5s", "10s", "1m"）
	TotalQPSWindows    map[string]*SlidingWindow            // 总QPS滑动窗口
	EndpointQPSWindows map[string]map[string]*SlidingWindow // 端点QPS滑动窗口，第一层key是时间窗口，第二层key是端点
	mu                 sync.RWMutex
}

// NewDailyStats 创建新的日统计
func NewDailyStats(date time.Time, qpsWindows []time.Duration) *DailyStats {
	ds := &DailyStats{
		Date:               date,
		EndpointStats:      make(map[string]int64),
		TotalQPSWindows:    make(map[string]*SlidingWindow),
		EndpointQPSWindows: make(map[string]map[string]*SlidingWindow),
	}

	// 初始化每个时间窗口的滑动窗口
	for _, window := range qpsWindows {
		windowStr := formatDuration(window)
		ds.TotalQPSWindows[windowStr] = NewSlidingWindow(window)
		ds.EndpointQPSWindows[windowStr] = make(map[string]*SlidingWindow)
	}

	return ds
}

// formatDuration 格式化时间间隔为字符串（如 "5s", "1m"）
func formatDuration(d time.Duration) string {
	seconds := int64(d.Seconds())
	minutes := int64(d.Minutes())
	hours := int64(d.Hours())

	if hours > 0 && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	if minutes > 0 && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", seconds)
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

// AddRequest 增加请求计数（用于QPS统计，使用滑动窗口）
func (d *DailyStats) AddRequest() {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	// 向所有时间窗口添加请求时间戳
	for _, window := range d.TotalQPSWindows {
		window.Add(now)
	}
}

// AddEndpointRequest 增加端点请求计数（用于QPS统计，使用滑动窗口）
func (d *DailyStats) AddEndpointRequest(endpoint string) {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	// 向所有时间窗口的该端点添加请求时间戳
	for windowStr, endpointWindows := range d.EndpointQPSWindows {
		// 如果端点窗口不存在，创建它
		if endpointWindows[endpoint] == nil {
			// 获取对应时间窗口的大小
			totalWindow := d.TotalQPSWindows[windowStr]
			if totalWindow != nil {
				endpointWindows[endpoint] = NewSlidingWindow(totalWindow.window)
			}
		}
		if endpointWindows[endpoint] != nil {
			endpointWindows[endpoint].Add(now)
		}
	}
}

// GetTotalQPS 获取总QPS（返回所有时间窗口的QPS）
// 返回格式: map[时间窗口]QPS值，如 {"5s": 100, "10s": 95, "1m": 80}
func (d *DailyStats) GetTotalQPS() map[string]int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]int64)
	for windowStr, window := range d.TotalQPSWindows {
		result[windowStr] = window.Count()
	}
	return result
}

// GetEndpointQPS 获取端点QPS统计（返回所有时间窗口的端点QPS）
// 返回格式: map[端点]map[时间窗口]QPS值
// 只返回QPS > 0的端点
func (d *DailyStats) GetEndpointQPS() map[string]map[string]int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]map[string]int64)

	// 遍历所有时间窗口
	for windowStr, endpointWindows := range d.EndpointQPSWindows {
		// 遍历该时间窗口下的所有端点
		for endpoint, window := range endpointWindows {
			if window != nil {
				count := window.Count()
				if count > 0 {
					// 初始化端点的map
					if result[endpoint] == nil {
						result[endpoint] = make(map[string]int64)
					}
					result[endpoint][windowStr] = count
				}
			}
		}
	}

	return result
}

// StatsManager 管理所有统计
type StatsManager struct {
	currentDay    *DailyStats
	ipStats       *IPStatsManager
	endpointRules []string        // 需要统计的端点规则，如 "/abc/def/**"
	qpsWindows    []time.Duration // QPS时间窗口列表
	mu            sync.RWMutex
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(endpointRules []string, qpsWindows []time.Duration) *StatsManager {
	now := time.Now()
	return &StatsManager{
		currentDay:    NewDailyStats(now, qpsWindows),
		ipStats:       NewIPStatsManager(),
		endpointRules: endpointRules,
		qpsWindows:    qpsWindows,
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
		m.currentDay = NewDailyStats(now, m.qpsWindows)
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
