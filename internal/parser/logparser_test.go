package parser

import (
	"testing"
)

func TestLogParser_Parse(t *testing.T) {
	parser := NewLogParser()

	testCases := []struct {
		name     string
		logLine  string
		expected *LogEntry
	}{
		{
			name:    "standard log line",
			logLine: `100.122.56.162 - - [11/Nov/2025:09:54:59 +0800] "POST /order2.0/a/order/getWholeList?ptype=3 HTTP/1.1" 200 516 2806 "http://www.sifangerp.cn/order2.0/a/order/Whole?cacheIds=2c90812d9a6e3c53019a7081e53a2535" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36"`,
			expected: &LogEntry{
				IP:        "100.122.56.162",
				BytesSent: 2806, // $bytes_sent is the second-to-last number
				Path:      "/order2.0/a/order/getWholeList",
				Timestamp: "11/Nov/2025:09:54:59 +0800",
			},
		},
		{
			name:    "log with different endpoint",
			logLine: `100.122.56.120 - - [11/Nov/2025:09:55:00 +0800] "POST /order2.0/a/order/getWholeList?ptype=3 HTTP/1.1" 200 1790 4080 "http://www.sifangerp.cn/order2.0/a/order/Whole" "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"`,
			expected: &LogEntry{
				IP:        "100.122.56.120",
				BytesSent: 4080,
				Path:      "/order2.0/a/order/getWholeList",
				Timestamp: "11/Nov/2025:09:55:00 +0800",
			},
		},
		{
			name:    "log with empty referer",
			logLine: `100.122.56.196 - - [11/Nov/2025:09:55:01 +0800] "POST /sferpAPI/api HTTP/1.1" 200 4620 4831 "-" "SifuBang/2.7.3 (iPhone; iOS 26.0.1; Scale/3.00)"`,
			expected: &LogEntry{
				IP:        "100.122.56.196",
				BytesSent: 4831,
				Path:      "/sferpAPI/api",
				Timestamp: "11/Nov/2025:09:55:01 +0800",
			},
		},
		{
			name:    "log with query parameters",
			logLine: `100.122.56.157 - - [11/Nov/2025:09:55:04 +0800] "POST /sferpAPI/api?v=1.0&method=common.uploadFile&messageFormat=json&appKey=00001 HTTP/1.1" 200 138 349 "-" "Dalvik/2.1.0 (Linux; U; Android 15; PKA110 Build/AP3A.240617.008)"`,
			expected: &LogEntry{
				IP:        "100.122.56.157",
				BytesSent: 349,
				Path:      "/sferpAPI/api",
				Timestamp: "11/Nov/2025:09:55:04 +0800",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.Parse(tc.logLine)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if result == nil {
				t.Fatal("Parse() returned nil")
			}

			if result.IP != tc.expected.IP {
				t.Errorf("IP = %v, want %v", result.IP, tc.expected.IP)
			}
			if result.BytesSent != tc.expected.BytesSent {
				t.Errorf("BytesSent = %v, want %v", result.BytesSent, tc.expected.BytesSent)
			}
			if result.Path != tc.expected.Path {
				t.Errorf("Path = %v, want %v", result.Path, tc.expected.Path)
			}
			if result.Timestamp != tc.expected.Timestamp {
				t.Errorf("Timestamp = %v, want %v", result.Timestamp, tc.expected.Timestamp)
			}
		})
	}
}

func TestLogParser_Parse_IgnoreInvalidLines(t *testing.T) {
	parser := NewLogParser()

	testCases := []struct {
		name    string
		logLine string
	}{
		{
			name:    "empty line should return nil without error",
			logLine: "",
		},
		{
			name:    "whitespace only line should return nil",
			logLine: "   \t  ",
		},
		{
			name:    "invalid format line should return nil",
			logLine: "this is not a valid nginx log line",
		},
		{
			name:    "line with invalid bytes_sent should return nil",
			logLine: `100.122.56.162 - - [11/Nov/2025:09:54:59 +0800] "POST /test HTTP/1.1" 200 516 invalid "referer" "user-agent"`,
		},
		{
			name:    "line with too few fields should return nil",
			logLine: "100.122.56.162 - -",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parser.Parse(tc.logLine)

			// 对于无法解析的行，应该返回 nil, nil（不返回错误）
			if err != nil {
				t.Errorf("Parse() should not return error for unparseable line, got error: %v", err)
			}
			if result != nil {
				t.Errorf("Parse() should return nil for unparseable line, got: %+v", result)
			}
		})
	}
}
