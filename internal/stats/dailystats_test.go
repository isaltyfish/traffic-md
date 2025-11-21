package stats

import (
	"testing"
	"time"
)

func TestMatchPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		// 测试 ** 匹配所有子路径
		{
			name:     "/a/b/**/* should match /a/b/c/d",
			path:     "/a/b/c/d",
			pattern:  "/a/b/**/*",
			expected: true,
		},
		{
			name:     "/a/b/**/* should match /a/b/ccc",
			path:     "/a/b/ccc",
			pattern:  "/a/b/**/*",
			expected: true,
		},
		{
			name:     "/a/b/**/* should match /a/b/c/d/e",
			path:     "/a/b/c/d/e",
			pattern:  "/a/b/**/*",
			expected: true,
		},
		{
			name:     "/a/b/** should match /a/b/c/d",
			path:     "/a/b/c/d",
			pattern:  "/a/b/**",
			expected: true,
		},
		{
			name:     "/a/b/** should match /a/b/ccc",
			path:     "/a/b/ccc",
			pattern:  "/a/b/**",
			expected: true,
		},
		{
			name:     "/a/b/** should match /a/b",
			path:     "/a/b",
			pattern:  "/a/b/**",
			expected: true,
		},

		// 测试 * 只匹配直接子路径
		{
			name:     "/a/b/* should match /a/b/ccc",
			path:     "/a/b/ccc",
			pattern:  "/a/b/*",
			expected: true,
		},
		{
			name:     "/a/b/* should NOT match /a/b/c/d",
			path:     "/a/b/c/d",
			pattern:  "/a/b/*",
			expected: false,
		},
		{
			name:     "/a/b/* should NOT match /a/b",
			path:     "/a/b",
			pattern:  "/a/b/*",
			expected: false,
		},

		// 测试精确匹配
		{
			name:     "exact match /a/b/c",
			path:     "/a/b/c",
			pattern:  "/a/b/c",
			expected: true,
		},
		{
			name:     "exact match /a/b should not match /a/b/c",
			path:     "/a/b/c",
			pattern:  "/a/b",
			expected: false,
		},

		// 测试混合模式
		{
			name:     "/api/*/users should match /api/v1/users",
			path:     "/api/v1/users",
			pattern:  "/api/*/users",
			expected: true,
		},
		{
			name:     "/api/*/users should NOT match /api/v1/users/123",
			path:     "/api/v1/users/123",
			pattern:  "/api/*/users",
			expected: false,
		},
		{
			name:     "/api/**/users should match /api/v1/users",
			path:     "/api/v1/users",
			pattern:  "/api/**/users",
			expected: true,
		},
		{
			name:     "/api/**/users should match /api/v1/v2/users",
			path:     "/api/v1/v2/users",
			pattern:  "/api/**/users",
			expected: true,
		},

		// 测试边界情况
		{
			name:     "root path /",
			path:     "/",
			pattern:  "/",
			expected: true,
		},
		{
			name:     "root path with **",
			path:     "/a",
			pattern:  "/**",
			expected: true,
		},
		{
			name:     "root path with *",
			path:     "/a",
			pattern:  "/*",
			expected: true,
		},
		{
			name:     "root path with * should not match /a/b",
			path:     "/a/b",
			pattern:  "/*",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchPath(tt.path, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchPath(%q, %q) = %v, expected %v", tt.path, tt.pattern, result, tt.expected)
			}
		})
	}
}

// TestQPSStatistics 测试QPS统计功能（基于滑动窗口）
func TestQPSStatistics(t *testing.T) {
	// 创建测试用的时间窗口：1秒和5秒
	qpsWindows := []time.Duration{1 * time.Second, 5 * time.Second}
	dailyStats := NewDailyStats(time.Now(), qpsWindows)

	// 测试总QPS
	t.Run("总QPS统计", func(t *testing.T) {
		// 添加10个请求
		for i := 0; i < 10; i++ {
			dailyStats.AddRequest()
		}

		// 验证总QPS（滑动窗口会自动计算）
		totalQPS := dailyStats.GetTotalQPS()
		if totalQPS["1s"] != 10 {
			t.Errorf("1s窗口的总QPS应该是10，实际是%d", totalQPS["1s"])
		}
		if totalQPS["5s"] != 10 {
			t.Errorf("5s窗口的总QPS应该是10，实际是%d", totalQPS["5s"])
		}

		// 等待1秒后，1s窗口应该清空，5s窗口应该还有
		time.Sleep(1100 * time.Millisecond)
		totalQPS = dailyStats.GetTotalQPS()
		if totalQPS["1s"] != 0 {
			t.Errorf("1s窗口的总QPS应该为0（已过期），实际是%d", totalQPS["1s"])
		}
		if totalQPS["5s"] != 10 {
			t.Errorf("5s窗口的总QPS应该仍为10，实际是%d", totalQPS["5s"])
		}
	})

	// 测试端点QPS
	t.Run("端点QPS统计", func(t *testing.T) {
		// 重置
		dailyStats = NewDailyStats(time.Now(), qpsWindows)

		// 添加不同端点的请求
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v2/orders")
		dailyStats.AddEndpointRequest("/api/v2/orders")

		// 验证端点QPS
		endpointQPS := dailyStats.GetEndpointQPS()
		if endpointQPS["/api/v1/users"]["1s"] != 3 {
			t.Errorf("/api/v1/users在1s窗口的QPS应该是3，实际是%d", endpointQPS["/api/v1/users"]["1s"])
		}
		if endpointQPS["/api/v2/orders"]["1s"] != 2 {
			t.Errorf("/api/v2/orders在1s窗口的QPS应该是2，实际是%d", endpointQPS["/api/v2/orders"]["1s"])
		}

		// 等待1秒后，1s窗口应该清空，但5s窗口还有值
		time.Sleep(1100 * time.Millisecond)
		endpointQPS = dailyStats.GetEndpointQPS()
		// 5s窗口应该还有值
		if endpointQPS["/api/v1/users"]["5s"] != 3 {
			t.Errorf("/api/v1/users在5s窗口的QPS应该仍为3，实际是%d", endpointQPS["/api/v1/users"]["5s"])
		}
		// 1s窗口应该为0（不返回）
		if _, exists := endpointQPS["/api/v1/users"]["1s"]; exists {
			t.Errorf("/api/v1/users在1s窗口的QPS应该为0（已过期），不应该出现在结果中")
		}
	})

	// 测试滑动窗口清理
	t.Run("滑动窗口清理", func(t *testing.T) {
		dailyStats = NewDailyStats(time.Now(), qpsWindows)

		// 添加请求
		dailyStats.AddRequest()
		dailyStats.AddEndpointRequest("/test")

		// 验证QPS有值
		totalQPS := dailyStats.GetTotalQPS()
		if totalQPS["1s"] != 1 {
			t.Errorf("总QPS在1s窗口应该是1，实际是%d", totalQPS["1s"])
		}

		// 等待超过1s窗口时间后，1s窗口应该过期，但5s窗口还有值
		time.Sleep(1100 * time.Millisecond)
		totalQPS = dailyStats.GetTotalQPS()
		if totalQPS["1s"] != 0 {
			t.Errorf("1s窗口的总QPS应该为0（已过期），实际是%d", totalQPS["1s"])
		}
		if totalQPS["5s"] != 1 {
			t.Errorf("5s窗口的总QPS应该仍为1，实际是%d", totalQPS["5s"])
		}

		endpointQPS := dailyStats.GetEndpointQPS()
		// 5s窗口应该还有值
		if endpointQPS["/test"]["5s"] != 1 {
			t.Errorf("/test在5s窗口的QPS应该仍为1，实际是%d", endpointQPS["/test"]["5s"])
		}
		// 1s窗口应该为0（不返回）
		if _, exists := endpointQPS["/test"]["1s"]; exists {
			t.Errorf("/test在1s窗口的QPS应该为0（已过期），不应该出现在结果中")
		}
	})
}
