package stats

import (
	"testing"
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
