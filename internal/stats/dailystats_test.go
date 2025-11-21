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

// TestQPSStatistics 测试QPS统计功能
func TestQPSStatistics(t *testing.T) {
	dailyStats := NewDailyStats(time.Now())

	// 测试总QPS
	t.Run("总QPS统计", func(t *testing.T) {
		// 添加10个请求
		for i := 0; i < 10; i++ {
			dailyStats.AddRequest()
		}

		// 更新QPS（模拟每秒更新）
		dailyStats.UpdateQPS()

		// 验证总QPS
		totalQPS := dailyStats.GetTotalQPS()
		if totalQPS != 10 {
			t.Errorf("总QPS应该是10，实际是%d", totalQPS)
		}

		// 再次添加5个请求
		for i := 0; i < 5; i++ {
			dailyStats.AddRequest()
		}

		// 更新QPS
		dailyStats.UpdateQPS()

		// 验证总QPS更新为5
		totalQPS = dailyStats.GetTotalQPS()
		if totalQPS != 5 {
			t.Errorf("总QPS应该是5，实际是%d", totalQPS)
		}
	})

	// 测试端点QPS
	t.Run("端点QPS统计", func(t *testing.T) {
		// 重置
		dailyStats = NewDailyStats(time.Now())

		// 添加不同端点的请求
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.AddEndpointRequest("/api/v2/orders")
		dailyStats.AddEndpointRequest("/api/v2/orders")

		// 更新QPS
		dailyStats.UpdateQPS()

		// 验证端点QPS
		endpointQPS := dailyStats.GetEndpointQPS()
		if endpointQPS["/api/v1/users"] != 3 {
			t.Errorf("/api/v1/users的QPS应该是3，实际是%d", endpointQPS["/api/v1/users"])
		}
		if endpointQPS["/api/v2/orders"] != 2 {
			t.Errorf("/api/v2/orders的QPS应该是2，实际是%d", endpointQPS["/api/v2/orders"])
		}

		// 下一秒只添加一个请求
		dailyStats.AddEndpointRequest("/api/v1/users")
		dailyStats.UpdateQPS()

		// 验证QPS更新
		endpointQPS = dailyStats.GetEndpointQPS()
		if endpointQPS["/api/v1/users"] != 1 {
			t.Errorf("/api/v1/users的QPS应该是1，实际是%d", endpointQPS["/api/v1/users"])
		}
		if _, exists := endpointQPS["/api/v2/orders"]; exists {
			t.Errorf("/api/v2/orders的QPS为0，不应该出现在结果中")
		}
	})

	// 测试QPS重置
	t.Run("QPS重置", func(t *testing.T) {
		dailyStats = NewDailyStats(time.Now())

		// 添加请求
		dailyStats.AddRequest()
		dailyStats.AddEndpointRequest("/test")

		// 更新QPS
		dailyStats.UpdateQPS()

		// 验证QPS有值
		if dailyStats.GetTotalQPS() != 1 {
			t.Errorf("总QPS应该是1")
		}

		// 不添加新请求，直接更新QPS
		dailyStats.UpdateQPS()

		// 验证QPS重置为0
		if dailyStats.GetTotalQPS() != 0 {
			t.Errorf("总QPS应该重置为0，实际是%d", dailyStats.GetTotalQPS())
		}

		endpointQPS := dailyStats.GetEndpointQPS()
		if len(endpointQPS) != 0 {
			t.Errorf("端点QPS应该为空，实际有%d个端点", len(endpointQPS))
		}
	})
}
