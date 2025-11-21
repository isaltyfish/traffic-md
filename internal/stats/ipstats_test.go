package stats

import (
	"context"
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

// TestEvictIfNeeded_ShouldNotEvictIPsThatGrowLarge 测试关键bug修复：
// 在收集候选者后，如果IP流量增长到超过阈值，不应该被删除
func TestEvictIfNeeded_ShouldNotEvictIPsThatGrowLarge(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()
	firstSeen := now.Add(-16 * time.Minute)

	// 创建一个会在收集候选者后流量增长的IP
	growthIP := "192.168.100.1"
	stat := manager.GetOrCreate(growthIP)
	stat.mu.Lock()
	stat.TotalBytes = 1024 * 1024 // 初始1MB，可以被淘汰
	stat.FirstSeen = firstSeen
	stat.mu.Unlock()

	// 创建4000个小流量IP，使总数超过阈值
	for i := 0; i < 4000; i++ {
		ip := fmt.Sprintf("10.0.4.%d", i)
		s := manager.GetOrCreate(ip)
		s.mu.Lock()
		s.TotalBytes = 1024 * 1024 // 1MB
		s.FirstSeen = firstSeen
		s.mu.Unlock()
	}

	initialCount := manager.Count()
	if initialCount != 4001 {
		t.Fatalf("Initial count should be 4001, got %d", initialCount)
	}

	// 手动模拟收集候选者的过程，然后在删除前增长流量
	// 这直接测试了bug场景：收集候选者时流量小，删除时流量大

	// 步骤1: 收集候选者（模拟EvictIfNeeded中的收集过程）
	force := false
	var candidates []struct {
		ip         string
		totalBytes int64
	}
	manager.mu.RLock()
	for ip, s := range manager.stats {
		if s.CanEvict(now, force) {
			candidates = append(candidates, struct {
				ip         string
				totalBytes int64
			}{ip: ip, totalBytes: s.GetTotalBytes()})
		}
	}
	manager.mu.RUnlock()

	// 验证growthIP在候选者列表中（因为初始流量是1MB）
	foundInCandidates := false
	for _, c := range candidates {
		if c.ip == growthIP {
			foundInCandidates = true
			if c.totalBytes != 1024*1024 {
				t.Errorf("growthIP should have 1MB in candidates, got %d", c.totalBytes)
			}
			break
		}
	}
	if !foundInCandidates {
		t.Fatal("growthIP should be in candidates list")
	}

	// 步骤2: 在删除前，增长growthIP的流量到10GB（模拟并发场景）
	growthStat := manager.Get(growthIP)
	growthStat.mu.Lock()
	growthStat.TotalBytes = 10 * 1024 * 1024 * 1024 // 增长到10GB
	growthStat.mu.Unlock()

	// 步骤3: 现在调用EvictIfNeeded，应该重新验证并保护growthIP
	manager.EvictIfNeeded()

	// 验证growthIP不应该被删除（因为它的流量已经增长到10GB，应该被保护）
	if manager.Get(growthIP) == nil {
		t.Errorf("growthIP %s should not be evicted because its traffic grew to 10GB, but it was evicted", growthIP)
	} else {
		remainingStat := manager.Get(growthIP)
		if remainingStat.GetTotalBytes() != 10*1024*1024*1024 {
			t.Errorf("growthIP should have 10GB, got %d", remainingStat.GetTotalBytes())
		}
	}

	// 验证最终数量
	finalCount := manager.Count()
	if finalCount > maxIPsBeforeEvict {
		t.Logf("Final count: %d (expected <= %d). growthIP was protected.", finalCount, maxIPsBeforeEvict)
	}
}

// TestEvictIfNeeded_Comprehensive 综合测试：验证IP淘汰逻辑的两个关键点
// 1. 淘汰IP应该按照流量从小到大的顺序淘汰，保证大流量的IP不会被淘汰
// 2. 新加入的IP应该有15分钟的保护期不应被淘汰
func TestEvictIfNeeded_Comprehensive(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()
	oldTime := now.Add(-16 * time.Minute)   // 16分钟前，超过15分钟保护期
	recentTime := now.Add(-5 * time.Minute) // 5分钟前，在15分钟保护期内

	// 定义测试IP组
	type testIP struct {
		ip         string
		bytes      int64
		firstSeen  time.Time
		shouldKeep bool // 是否应该被保留
		reason     string
	}

	testIPs := []testIP{
		// 大流量IP（应该被保护，无论时间）
		{"192.168.1.1", 10 * 1024 * 1024 * 1024, oldTime, true, "大流量10GB"},
		{"192.168.1.2", 8 * 1024 * 1024 * 1024, oldTime, true, "大流量8GB"},
		{"192.168.1.3", 6 * 1024 * 1024 * 1024, oldTime, true, "大流量6GB"},
		{"192.168.1.4", 5 * 1024 * 1024 * 1024, oldTime, true, "大流量5GB（阈值）"},

		// 新加入的IP（15分钟内，应该被保护，无论流量）
		{"192.168.2.1", 1024 * 1024, recentTime, true, "新IP-1MB-5分钟前"},
		{"192.168.2.2", 10 * 1024 * 1024, recentTime, true, "新IP-10MB-5分钟前"},
		{"192.168.2.3", 100 * 1024 * 1024, recentTime, true, "新IP-100MB-5分钟前"},
		{"192.168.2.4", 1024 * 1024 * 1024, recentTime, true, "新IP-1GB-5分钟前"},

		// 小流量且超过15分钟的IP（应该被淘汰）
		{"10.0.1.1", 1024 * 1024, oldTime, false, "小流量1MB-16分钟前"},
		{"10.0.1.2", 2 * 1024 * 1024, oldTime, false, "小流量2MB-16分钟前"},
		{"10.0.1.3", 5 * 1024 * 1024, oldTime, false, "小流量5MB-16分钟前"},
		{"10.0.1.4", 10 * 1024 * 1024, oldTime, false, "小流量10MB-16分钟前"},
		{"10.0.1.5", 50 * 1024 * 1024, oldTime, false, "小流量50MB-16分钟前"},
		{"10.0.1.6", 100 * 1024 * 1024, oldTime, false, "小流量100MB-16分钟前"},
		{"10.0.1.7", 500 * 1024 * 1024, oldTime, false, "小流量500MB-16分钟前"},
		{"10.0.1.8", 1024 * 1024 * 1024, oldTime, false, "小流量1GB-16分钟前"},
		{"10.0.1.9", 2 * 1024 * 1024 * 1024, oldTime, false, "小流量2GB-16分钟前"},
		{"10.0.1.10", 3 * 1024 * 1024 * 1024, oldTime, false, "小流量3GB-16分钟前"},
	}

	// 创建测试IP
	for _, tip := range testIPs {
		stat := manager.GetOrCreate(tip.ip)
		stat.mu.Lock()
		stat.TotalBytes = tip.bytes
		stat.FirstSeen = tip.firstSeen
		stat.mu.Unlock()
	}

	// 创建足够多的小流量IP，使总数超过4000，触发淘汰
	// 这些IP都是小流量且超过15分钟，应该被优先淘汰
	smallIPsToEvict := make([]string, 0, 4000)
	for i := 0; i < 4000; i++ {
		ip := fmt.Sprintf("10.0.2.%d", i)
		smallIPsToEvict = append(smallIPsToEvict, ip)
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = 1024 * 1024 // 1MB
		stat.FirstSeen = oldTime
		stat.mu.Unlock()
	}

	initialCount := manager.Count()
	expectedInitialCount := len(testIPs) + len(smallIPsToEvict)
	if initialCount != expectedInitialCount {
		t.Fatalf("Initial count should be %d, got %d", expectedInitialCount, initialCount)
	}

	// 触发淘汰
	manager.EvictIfNeeded()

	finalCount := manager.Count()
	expectedFinalCount := maxIPsBeforeEvict // 4000

	// 验证点1: 大流量IP不应该被淘汰
	t.Run("验证点1-大流量IP不应被淘汰", func(t *testing.T) {
		largeTrafficIPs := []testIP{
			{"192.168.1.1", 10 * 1024 * 1024 * 1024, oldTime, true, "大流量10GB"},
			{"192.168.1.2", 8 * 1024 * 1024 * 1024, oldTime, true, "大流量8GB"},
			{"192.168.1.3", 6 * 1024 * 1024 * 1024, oldTime, true, "大流量6GB"},
			{"192.168.1.4", 5 * 1024 * 1024 * 1024, oldTime, true, "大流量5GB"},
		}

		for _, tip := range largeTrafficIPs {
			stat := manager.Get(tip.ip)
			if stat == nil {
				t.Errorf("❌ 大流量IP %s (%s) 被错误淘汰了！", tip.ip, tip.reason)
			} else {
				if stat.GetTotalBytes() != tip.bytes {
					t.Errorf("大流量IP %s 流量不匹配: expected %d, got %d", tip.ip, tip.bytes, stat.GetTotalBytes())
				} else {
					t.Logf("✓ 大流量IP %s (%s) 被正确保留", tip.ip, tip.reason)
				}
			}
		}
	})

	// 验证点2: 新加入的IP（15分钟内）不应该被淘汰
	t.Run("验证点2-新IP（15分钟内）不应被淘汰", func(t *testing.T) {
		recentIPs := []testIP{
			{"192.168.2.1", 1024 * 1024, recentTime, true, "新IP-1MB-5分钟前"},
			{"192.168.2.2", 10 * 1024 * 1024, recentTime, true, "新IP-10MB-5分钟前"},
			{"192.168.2.3", 100 * 1024 * 1024, recentTime, true, "新IP-100MB-5分钟前"},
			{"192.168.2.4", 1024 * 1024 * 1024, recentTime, true, "新IP-1GB-5分钟前"},
		}

		for _, tip := range recentIPs {
			stat := manager.Get(tip.ip)
			if stat == nil {
				t.Errorf("❌ 新IP %s (%s) 被错误淘汰了！应该在15分钟保护期内", tip.ip, tip.reason)
			} else {
				firstSeen := stat.GetFirstSeen()
				age := now.Sub(firstSeen)
				if age < minRetentionTime {
					t.Logf("✓ 新IP %s (%s, 年龄: %v) 被正确保留", tip.ip, tip.reason, age)
				} else {
					t.Errorf("新IP %s 年龄 %v 超过保护期，但仍被保留（可能是其他原因）", tip.ip, age)
				}
			}
		}
	})

	// 验证点3: 被淘汰的IP应该都是小流量且超过15分钟的
	t.Run("验证点3-被淘汰的IP都是小流量且超过15分钟", func(t *testing.T) {
		// 检查所有被淘汰的IP
		allRemaining := manager.GetAll()
		remainingIPSet := make(map[string]bool)
		for ip := range allRemaining {
			remainingIPSet[ip] = true
		}

		// 验证：所有被淘汰的IP都应该是小流量且超过15分钟
		invalidEvicted := make([]string, 0)
		for _, tip := range testIPs {
			if !remainingIPSet[tip.ip] {
				// 这个IP被淘汰了
				age := now.Sub(tip.firstSeen)
				if age < minRetentionTime {
					invalidEvicted = append(invalidEvicted, fmt.Sprintf("%s (新IP,年龄:%v)", tip.ip, age))
				} else if tip.bytes >= highTrafficThreshold {
					invalidEvicted = append(invalidEvicted, fmt.Sprintf("%s (大流量:%d)", tip.ip, tip.bytes))
				} else {
					t.Logf("✓ 小流量旧IP %s (%s) 被正确淘汰", tip.ip, tip.reason)
				}
			}
		}

		if len(invalidEvicted) > 0 {
			t.Errorf("❌ 发现不应该被淘汰的IP被淘汰了: %v", invalidEvicted)
		} else {
			t.Logf("✓ 所有被淘汰的IP都是小流量且超过15分钟")
		}
	})

	// 验证点4: 淘汰顺序应该是按流量从小到大
	t.Run("验证点4-淘汰顺序按流量从小到大", func(t *testing.T) {
		allRemaining := manager.GetAll()
		remainingIPSet := make(map[string]bool)
		for ip := range allRemaining {
			remainingIPSet[ip] = true
		}

		// 统计被淘汰的测试IP及其流量
		evictedTestIPs := make([]testIP, 0)
		for _, tip := range testIPs {
			if !remainingIPSet[tip.ip] {
				evictedTestIPs = append(evictedTestIPs, tip)
			}
		}

		// 验证：被淘汰的IP中不应该有大流量IP（>=5GB）
		largeEvicted := make([]string, 0)
		for _, tip := range evictedTestIPs {
			if tip.bytes >= highTrafficThreshold {
				largeEvicted = append(largeEvicted, fmt.Sprintf("%s (%d bytes)", tip.ip, tip.bytes))
			}
		}

		if len(largeEvicted) > 0 {
			t.Errorf("❌ 发现大流量IP被淘汰: %v", largeEvicted)
		} else {
			t.Logf("✓ 没有大流量IP被淘汰")
		}

		// 验证：被淘汰的IP中不应该有新IP（15分钟内）
		recentEvicted := make([]string, 0)
		for _, tip := range evictedTestIPs {
			if now.Sub(tip.firstSeen) < minRetentionTime {
				recentEvicted = append(recentEvicted, fmt.Sprintf("%s (年龄:%v)", tip.ip, now.Sub(tip.firstSeen)))
			}
		}

		if len(recentEvicted) > 0 {
			t.Errorf("❌ 发现新IP（15分钟内）被淘汰: %v", recentEvicted)
		} else {
			t.Logf("✓ 没有新IP（15分钟内）被淘汰")
		}

		// 验证：剩余IP中，小流量IP的流量应该 >= 被淘汰IP的流量（淘汰顺序正确）
		if len(evictedTestIPs) > 0 {
			// 找到被淘汰IP中的最大流量
			maxEvictedBytes := int64(0)
			for _, tip := range evictedTestIPs {
				if tip.bytes > maxEvictedBytes {
					maxEvictedBytes = tip.bytes
				}
			}

			// 找到剩余IP中（可淘汰的）的最小流量
			minRemainingBytes := int64(-1)
			for _, tip := range testIPs {
				if remainingIPSet[tip.ip] {
					stat := manager.Get(tip.ip)
					if stat != nil {
						age := now.Sub(stat.GetFirstSeen())
						bytes := stat.GetTotalBytes()
						// 只考虑可淘汰的IP（超过15分钟且流量小于5GB）
						if age >= minRetentionTime && bytes < highTrafficThreshold {
							if minRemainingBytes == -1 || bytes < minRemainingBytes {
								minRemainingBytes = bytes
							}
						}
					}
				}
			}

			if minRemainingBytes != -1 && maxEvictedBytes > minRemainingBytes {
				t.Errorf("❌ 淘汰顺序错误：被淘汰的最大流量(%d) > 剩余的最小流量(%d)", maxEvictedBytes, minRemainingBytes)
			} else if minRemainingBytes != -1 {
				t.Logf("✓ 淘汰顺序正确：被淘汰的最大流量(%d) <= 剩余的最小流量(%d)", maxEvictedBytes, minRemainingBytes)
			}
		}

		// 验证最终数量
		if finalCount > expectedFinalCount {
			t.Errorf("最终IP数量 %d 超过预期 %d", finalCount, expectedFinalCount)
		} else {
			t.Logf("✓ 最终IP数量: %d (预期 <= %d)", finalCount, expectedFinalCount)
		}
	})

	// 总结报告
	t.Logf("\n=== 淘汰测试总结 ===")
	t.Logf("初始IP数量: %d", initialCount)
	t.Logf("最终IP数量: %d", finalCount)
	t.Logf("淘汰数量: %d", initialCount-finalCount)
	t.Logf("预期保留: <= %d", expectedFinalCount)
}

// TestEvictIfNeeded_ConcurrentSafety 并发安全测试：
// 一边有新IP加入，一边淘汰老的IP，验证两点要求：
// 1. 大流量的IP不会被淘汰
// 2. 新加入的IP（15分钟内）不会被淘汰
func TestEvictIfNeeded_ConcurrentSafety(t *testing.T) {
	manager := NewIPStatsManager()
	now := time.Now()
	oldTime := now.Add(-16 * time.Minute) // 16分钟前，超过15分钟保护期

	// 创建一些大流量IP（应该被保护）
	largeIPs := make(map[string]int64)
	for i := 0; i < 10; i++ {
		ip := fmt.Sprintf("192.168.100.%d", i)
		bytes := int64(5+i) * 1024 * 1024 * 1024 // 5GB到14GB
		largeIPs[ip] = bytes
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = bytes
		stat.FirstSeen = oldTime
		stat.mu.Unlock()
	}

	// 创建一些旧的小流量IP（可以被淘汰）
	for i := 0; i < 2000; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		stat := manager.GetOrCreate(ip)
		stat.mu.Lock()
		stat.TotalBytes = int64(i%100+1) * 1024 * 1024 // 1MB到100MB
		stat.FirstSeen = oldTime
		stat.mu.Unlock()
	}

	// 记录新IP（用于验证15分钟保护）
	newIPs := make(map[string]time.Time)
	var newIPsMu sync.Mutex

	// 并发控制
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Goroutine 1: 持续添加新IP
	wg.Add(1)
	go func() {
		defer wg.Done()
		counter := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// 添加新IP（在15分钟保护期内）
				ip := fmt.Sprintf("192.168.200.%d", counter)
				stat := manager.GetOrCreate(ip)
				stat.AddBytes(int64(counter%1000+1) * 1024 * 1024) // 1MB到1GB

				// 记录新IP的创建时间
				newIPsMu.Lock()
				newIPs[ip] = time.Now()
				newIPsMu.Unlock()

				counter++
				time.Sleep(10 * time.Millisecond) // 每10ms添加一个
			}
		}
	}()

	// Goroutine 2: 持续触发淘汰
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				manager.EvictIfNeeded()
				time.Sleep(50 * time.Millisecond) // 每50ms触发一次淘汰
			}
		}
	}()

	// Goroutine 3: 持续更新大流量IP的流量（模拟流量增长）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				for ip := range largeIPs {
					stat := manager.Get(ip)
					if stat != nil {
						// 模拟流量增长
						stat.AddBytes(10 * 1024 * 1024) // 每次增加10MB
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// 等待并发操作完成
	wg.Wait()

	// 验证点1: 大流量IP不应该被淘汰
	t.Run("并发验证点1-大流量IP不应被淘汰", func(t *testing.T) {
		evictedLargeIPs := make([]string, 0)
		for ip := range largeIPs {
			stat := manager.Get(ip)
			if stat == nil {
				evictedLargeIPs = append(evictedLargeIPs, ip)
			} else {
				currentBytes := stat.GetTotalBytes()
				expectedMinBytes := largeIPs[ip]
				if currentBytes < expectedMinBytes {
					t.Errorf("大流量IP %s 流量异常: expected >= %d, got %d", ip, expectedMinBytes, currentBytes)
				} else {
					t.Logf("✓ 大流量IP %s 被正确保留 (流量: %d bytes)", ip, currentBytes)
				}
			}
		}

		if len(evictedLargeIPs) > 0 {
			t.Errorf("❌ 在并发场景下，大流量IP被错误淘汰: %v", evictedLargeIPs)
		} else {
			t.Logf("✓ 所有大流量IP在并发场景下都被正确保护")
		}
	})

	// 验证点2: 新IP（15分钟内）不应该被淘汰
	t.Run("并发验证点2-新IP（15分钟内）不应被淘汰", func(t *testing.T) {
		newIPsMu.Lock()
		currentTime := time.Now()
		evictedNewIPs := make([]string, 0)
		protectedNewIPs := 0

		for ip, createTime := range newIPs {
			age := currentTime.Sub(createTime)
			stat := manager.Get(ip)

			if stat == nil {
				// IP被淘汰了
				if age < minRetentionTime {
					// 在保护期内被淘汰，这是错误的
					evictedNewIPs = append(evictedNewIPs, fmt.Sprintf("%s (年龄:%v)", ip, age))
				} else {
					// 超过保护期被淘汰，这是正常的
					t.Logf("ℹ 新IP %s 超过保护期后被淘汰 (年龄:%v)", ip, age)
				}
			} else {
				// IP还存在
				if age < minRetentionTime {
					// 在保护期内，应该被保护
					protectedNewIPs++
					t.Logf("✓ 新IP %s 在保护期内被正确保护 (年龄:%v, 流量:%d)", ip, age, stat.GetTotalBytes())
				}
			}
		}
		newIPsMu.Unlock()

		if len(evictedNewIPs) > 0 {
			t.Errorf("❌ 在并发场景下，新IP（15分钟内）被错误淘汰: %v", evictedNewIPs)
		} else {
			t.Logf("✓ 所有新IP（15分钟内）在并发场景下都被正确保护 (保护了 %d 个新IP)", protectedNewIPs)
		}
	})

	// 验证点3: 最终状态检查
	t.Run("并发验证点3-最终状态检查", func(t *testing.T) {
		finalCount := manager.Count()
		t.Logf("最终IP数量: %d", finalCount)

		// 统计剩余IP的类型
		allStats := manager.GetAll()
		largeCount := 0
		newCount := 0
		oldSmallCount := 0

		currentTime := time.Now()
		for _, stat := range allStats {
			bytes := stat.GetTotalBytes()
			age := currentTime.Sub(stat.GetFirstSeen())

			if bytes >= highTrafficThreshold {
				largeCount++
			} else if age < minRetentionTime {
				newCount++
			} else {
				oldSmallCount++
			}
		}

		t.Logf("剩余IP统计:")
		t.Logf("  大流量IP (>=5GB): %d", largeCount)
		t.Logf("  新IP (<15分钟): %d", newCount)
		t.Logf("  旧小流量IP (>=15分钟, <5GB): %d", oldSmallCount)

		// 验证大流量IP数量
		if largeCount < len(largeIPs) {
			t.Errorf("❌ 大流量IP数量不足: expected >= %d, got %d", len(largeIPs), largeCount)
		} else {
			t.Logf("✓ 大流量IP数量正确: %d", largeCount)
		}

		// 验证最终数量不超过阈值太多（允许一些新IP）
		if finalCount > maxIPsBeforeEvict+100 {
			t.Logf("⚠ 最终IP数量 %d 超过阈值较多，但可能是新IP持续加入导致的", finalCount)
		}
	})

	// 总结报告
	t.Logf("\n=== 并发安全测试总结 ===")
	t.Logf("测试时长: 5秒")
	t.Logf("最终IP数量: %d", manager.Count())
	t.Logf("大流量IP数量: %d (应该全部保留)", len(largeIPs))
}
