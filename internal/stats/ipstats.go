package stats

import (
	"sort"
	"sync"
	"time"
)

const (
	// 淘汰策略常量
	minRetentionTime     = 15 * time.Minute       // IP至少保留15分钟
	highTrafficThreshold = 5 * 1024 * 1024 * 1024 // 5GB
	maxIPsBeforeEvict    = 4000                   // 超过1000个IP触发淘汰
	forceEvictThreshold  = 5000                   // 超过2000个IP强制淘汰
)

// IPStats 存储单个IP的统计信息
type IPStats struct {
	IP          string
	TotalBytes  int64
	FirstSeen   time.Time
	LastSeen    time.Time
	AccessCount int64 // 访问次数
	mu          sync.RWMutex
}

// AddBytes 增加流量统计
func (s *IPStats) AddBytes(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalBytes += bytes
	s.LastSeen = time.Now()
	s.AccessCount++
	if s.FirstSeen.IsZero() {
		s.FirstSeen = time.Now()
	}
}

// GetTotalBytes 获取总流量
func (s *IPStats) GetTotalBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalBytes
}

// GetAccessCount 获取访问次数
func (s *IPStats) GetAccessCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AccessCount
}

// GetFirstSeen 获取首次出现时间
func (s *IPStats) GetFirstSeen() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.FirstSeen
}

// CanEvict 判断是否可以淘汰
func (s *IPStats) CanEvict(now time.Time, force bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 强制淘汰时，只保护超过5GB的IP
	if force {
		return s.TotalBytes < highTrafficThreshold
	}

	// 条件1: 至少保留15分钟
	if now.Sub(s.FirstSeen) < minRetentionTime {
		return false
	}

	// 条件2: 流量超过5GB当天不淘汰
	if s.TotalBytes >= highTrafficThreshold {
		return false
	}

	// 条件3: 其他情况可以淘汰（由LFU决定）
	return true
}

// IPStatsManager 管理所有IP的统计
type IPStatsManager struct {
	stats map[string]*IPStats
	mu    sync.RWMutex
}

// NewIPStatsManager 创建新的IP统计管理器
func NewIPStatsManager() *IPStatsManager {
	return &IPStatsManager{
		stats: make(map[string]*IPStats),
	}
}

// GetOrCreate 获取或创建IP统计
func (m *IPStatsManager) GetOrCreate(ip string) *IPStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stat, exists := m.stats[ip]; exists {
		return stat
	}

	stat := &IPStats{
		IP: ip,
	}
	m.stats[ip] = stat
	return stat
}

// Get 获取IP统计（如果不存在返回nil）
func (m *IPStatsManager) Get(ip string) *IPStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats[ip]
}

// GetAll 获取所有IP统计
func (m *IPStatsManager) GetAll() map[string]*IPStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*IPStats)
	for k, v := range m.stats {
		result[k] = v
	}
	return result
}

// Count 获取IP数量
func (m *IPStatsManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.stats)
}

// EvictIfNeeded 如果需要，执行淘汰策略
func (m *IPStatsManager) EvictIfNeeded() {
	count := m.Count()
	if count <= maxIPsBeforeEvict {
		return
	}

	force := count >= forceEvictThreshold
	now := time.Now()

	// 收集可淘汰的IP及其流量
	type evictCandidate struct {
		ip         string
		totalBytes int64
	}

	var candidates []evictCandidate
	m.mu.RLock()
	for ip, stat := range m.stats {
		if stat.CanEvict(now, force) {
			candidates = append(candidates, evictCandidate{
				ip:         ip,
				totalBytes: stat.GetTotalBytes(),
			})
		}
	}
	m.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	// 按流量从小到大排序（流量小的优先淘汰）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].totalBytes < candidates[j].totalBytes
	})

	m.mu.Lock()
	defer m.mu.Unlock()

	// 重新获取当前IP数量（因为在收集候选者后可能有新IP被添加）
	currentCount := len(m.stats)
	if currentCount <= maxIPsBeforeEvict {
		return
	}

	// 重新计算需要淘汰的数量
	evictCount := currentCount - maxIPsBeforeEvict
	if evictCount <= 0 {
		return
	}

	// 重新验证候选者并重新排序（因为IP状态可能在收集后已变化）
	// 关键修复：必须重新验证每个候选者是否仍然可以淘汰
	validCandidates := make([]evictCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		stat, exists := m.stats[candidate.ip]
		if !exists {
			// IP已被删除，跳过
			continue
		}

		// 重新验证IP是否仍然可以淘汰（重要：IP状态可能已变化）
		// 例如：在收集候选者时流量是1MB，但现在可能已增长到10GB，应该被保护
		if !stat.CanEvict(now, force) {
			// IP状态已变化，不再可以淘汰，跳过
			continue
		}

		// 重新获取当前流量（因为流量可能已变化）
		currentBytes := stat.GetTotalBytes()
		validCandidates = append(validCandidates, evictCandidate{
			ip:         candidate.ip,
			totalBytes: currentBytes,
		})
	}

	// 如果有效候选者数量不足，只能淘汰可用的
	if len(validCandidates) == 0 {
		return
	}

	// 基于最新流量重新排序
	sort.Slice(validCandidates, func(i, j int) bool {
		return validCandidates[i].totalBytes < validCandidates[j].totalBytes
	})

	// 限制淘汰数量不超过有效候选者数量
	if evictCount > len(validCandidates) {
		evictCount = len(validCandidates)
	}

	// 淘汰流量最小的IP
	for i := 0; i < evictCount; i++ {
		delete(m.stats, validCandidates[i].ip)
	}
}

// Reset 重置所有统计（用于跨天）
func (m *IPStatsManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = make(map[string]*IPStats)
}
