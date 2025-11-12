package stats

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestEvictIfNeeded_ShouldEvictSmallTrafficIPs 测试淘汰策略：应该淘汰流量小的IP，保留流量大的IP
func TestEvictIfNeeded_ShouldEvictSmallTrafficIPs(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()

	// 创建超过4000个IP，其中一些流量很大，一些流量很小
	// 设置FirstSeen为16分钟前，确保可以淘汰
	firstSeen := now.Add(-16 * time.Minute)

	// 创建10个大流量IP（每个10GB，应该被保护）
	largeIPs := make([]string, 10)
	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("192.168.1.%d", i)
		largeIPs[i] = ip
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 10 * 1024 * 1024 * 1024 // 10GB
		stat.FirstSeen = firstSeen
		stat.mu.Unlock()
	}

	// 创建4000个小流量IP（每个1MB，应该被淘汰）
	smallIPs := make([]string, 4000)
	for i := 0; i < 4000; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		smallIPs[i] = ip
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024 // 1MB
		stat.FirstSeen = firstSeen
		stat.mu.Unlock()
	}

	initialCount := manager.Count()
	if initialCount != 4010 {
		t.Fatalf("Initial count should be 4010, got %d", initialCount)
	}

	// 触发淘汰
	manager.EvictIfNeeded()

	// 验证大流量IP应该都被保留
	for _, ip := range largeIPs {
		if stat := manager.Get(ip); stat == nil {
			t.Errorf("Large traffic IP %s should not be evicted, but it was", ip)
		} else if stat.GetTotalBytes() != 10*1024*1024*1024 {
			t.Errorf("Large traffic IP %s should have 10GB, got %d", ip, stat.GetTotalBytes())
		}
	}

	// 验证小流量IP应该被淘汰（至少淘汰一部分）
	finalCount := manager.Count()
	if finalCount > maxIPsBeforeEvict {
		t.Errorf("After eviction, IP count should be <= %d, got %d", maxIPsBeforeEvict, finalCount)
	}

	// 验证被淘汰的IP都是小流量的，大流量IP不应该被淘汰
	allStats := manager.GetAll()
	smallIPsRemaining := 0
	for ip, stat := range allStats {
		// 检查是否是小流量IP（1MB）
		if stat.GetTotalBytes() == 1024*1024 {
			smallIPsRemaining++
		}
		// 验证大流量IP不应该被淘汰
		if stat.GetTotalBytes() == 10*1024*1024*1024 {
			// 大流量IP应该都在largeIPs列表中
			found := false
			for _, largeIP := range largeIPs {
				if largeIP == ip {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Found unexpected large traffic IP %s (10GB) that should not exist", ip)
			}
		}
	}

	// 验证淘汰后的数量
	expectedRemaining := 10 + (4000 - (initialCount - maxIPsBeforeEvict)) // 10个大流量 + 剩余的小流量
	actualRemaining := len(allStats)
	if actualRemaining != expectedRemaining && actualRemaining > maxIPsBeforeEvict {
		t.Logf("Info: After eviction, %d IPs remain (expected around %d). Small IPs remaining: %d",
			actualRemaining, expectedRemaining, smallIPsRemaining)
	}
}

// TestEvictIfNeeded_ShouldNotEvictRecentIPs 测试15分钟保护策略
func TestEvictIfNeeded_ShouldNotEvictRecentIPs(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()

	// 创建超过4000个IP
	// 一些是15分钟内的（应该被保护）
	// 一些是16分钟前的（可以被淘汰）

	recentIPs := make([]string, 100)
	oldIPs := make([]string, 4000)

	// 创建100个15分钟内的IP（应该被保护）
	for i := 0; i < 100; i++ {
		ip := fmt.Sprintf("192.168.2.%d", i)
		recentIPs[i] = ip
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024               // 1MB
		stat.FirstSeen = now.Add(-10 * time.Minute) // 10分钟前
		stat.mu.Unlock()
	}

	// 创建4000个16分钟前的IP（可以被淘汰）
	for i := 0; i < 4000; i++ {
		ip := fmt.Sprintf("10.0.1.%d", i)
		oldIPs[i] = ip
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024               // 1MB
		stat.FirstSeen = now.Add(-16 * time.Minute) // 16分钟前
		stat.mu.Unlock()
	}

	// 触发淘汰
	manager.EvictIfNeeded()

	// 验证15分钟内的IP应该都被保留
	for _, ip := range recentIPs {
		if stat := manager.Get(ip); stat == nil {
			t.Errorf("Recent IP %s (within 15 minutes) should not be evicted, but it was", ip)
		}
	}
}

// TestEvictIfNeeded_ConcurrentAccess 测试并发访问时的安全性
func TestEvictIfNeeded_ConcurrentAccess(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()
	firstSeen := now.Add(-16 * time.Minute)

	// 创建4000个IP
	for i := 0; i < 4000; i++ {
		ip := fmt.Sprintf("10.0.2.%d", i)
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024 // 1MB
		stat.FirstSeen = firstSeen
		stat.mu.Unlock()
	}

	// 并发添加新IP和触发淘汰
	var wg sync.WaitGroup
	wg.Add(2)

	// goroutine 1: 持续添加新IP
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			ip := fmt.Sprintf("192.168.3.%d", i)
			stat := manager.GetOrCreate(ip)
			stat.AddBytes(1024 * 1024)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// goroutine 2: 触发淘汰
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			manager.EvictIfNeeded()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()

	// 验证最终状态是合理的
	finalCount := manager.Count()
	if finalCount > forceEvictThreshold {
		t.Errorf("After concurrent operations, IP count should be <= %d, got %d", forceEvictThreshold, finalCount)
	}
}

// TestGetAll_Sorting 测试GetAll返回的数据是否能正确排序
func TestGetAll_Sorting(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()

	// 创建多个IP，流量不同
	ips := []struct {
		ip    string
		bytes int64
	}{
		{"192.168.1.1", 100 * 1024 * 1024}, // 100MB
		{"192.168.1.2", 50 * 1024 * 1024},  // 50MB
		{"192.168.1.3", 200 * 1024 * 1024}, // 200MB
		{"192.168.1.4", 10 * 1024 * 1024},  // 10MB
		{"192.168.1.5", 150 * 1024 * 1024}, // 150MB
	}

	for _, item := range ips {
		stat := manager.GetOrCreate(item.ip)
		stat.mu.Lock()
		stat.TotalBytes = item.bytes
		stat.FirstSeen = now
		stat.mu.Unlock()
	}

	// 获取所有统计并按流量排序
	allStats := manager.GetAll()
	type ipStat struct {
		ip    string
		bytes int64
	}
	statsList := make([]ipStat, 0, len(allStats))
	for ip, stat := range allStats {
		statsList = append(statsList, ipStat{
			ip:    ip,
			bytes: stat.GetTotalBytes(),
		})
	}

	// 按流量从大到小排序
	sort.Slice(statsList, func(i, j int) bool {
		return statsList[i].bytes > statsList[j].bytes
	})

	// 验证排序正确
	expectedOrder := []int64{200 * 1024 * 1024, 150 * 1024 * 1024, 100 * 1024 * 1024, 50 * 1024 * 1024, 10 * 1024 * 1024}
	for i, expected := range expectedOrder {
		if statsList[i].bytes != expected {
			t.Errorf("Sorting error: position %d should have %d bytes, got %d", i, expected, statsList[i].bytes)
		}
	}
}

// TestEvictIfNeeded_CountCalculation 测试淘汰数量计算是否正确
func TestEvictIfNeeded_CountCalculation(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()
	firstSeen := now.Add(-16 * time.Minute)

	// 创建恰好4001个IP（刚好超过阈值）
	for i := 0; i < 4001; i++ {
		ip := fmt.Sprintf("10.0.3.%d", i)
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024 // 1MB
		stat.FirstSeen = firstSeen
		stat.mu.Unlock()
	}

	initialCount := manager.Count()
	if initialCount != 4001 {
		t.Fatalf("Initial count should be 4001, got %d", initialCount)
	}

	// 触发淘汰
	manager.EvictIfNeeded()

	finalCount := manager.Count()
	expectedCount := maxIPsBeforeEvict // 4000
	if finalCount != expectedCount {
		t.Errorf("After eviction, count should be %d, got %d", expectedCount, finalCount)
	}
}
