package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// LogEntry 表示解析后的nginx日志条目
type LogEntry struct {
	IP        string
	BytesSent int64
	Path      string
	Timestamp string
}

// LogParser 解析nginx日志
type LogParser struct {
	// nginx日志格式通常是：
	// $remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"
	// 或者自定义格式包含 $bytes_sent
	pattern *regexp.Regexp
}

// NewLogParser 创建新的日志解析器
// 支持常见的nginx日志格式，尝试匹配包含bytes_sent的字段
// 标准格式: $remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent $bytes_sent "$http_referer" "$http_user_agent"
func NewLogParser() *LogParser {
	// 匹配标准nginx日志格式: IP - - [timestamp] "request" status body_bytes_sent bytes_sent ...
	// 需要匹配两个数字字段，第二个是 $bytes_sent
	pattern := regexp.MustCompile(`^(\S+)\s+.*?\[([^\]]+)\].*?"([^"]+)".*?\s+(\d+)\s+(\d+)\s+(\d+)`)
	return &LogParser{
		pattern: pattern,
	}
}

// Parse 解析单行日志
// 返回值: (LogEntry, error)
// - 如果解析成功，返回 (LogEntry, nil)
// - 如果无法解析（空行、格式不匹配等），返回 (nil, nil)，不会返回错误
// 这样设计是为了让调用者可以忽略无法处理的行，而不影响整体统计
func (p *LogParser) Parse(line string) (*LogEntry, error) {
	line = strings.TrimSpace(line)
	// 忽略空行
	if line == "" {
		return nil, nil
	}

	// 尝试使用正则表达式解析
	matches := p.pattern.FindStringSubmatch(line)
	if len(matches) >= 7 {
		ip := matches[1]
		timestamp := matches[2]
		request := matches[3]
		status := matches[4]
		bodyBytesSentStr := matches[5] // $body_bytes_sent
		bytesSentStr := matches[6]     // $bytes_sent (这是我们需要的)

		bytesSent, err := strconv.ParseInt(bytesSentStr, 10, 64)
		if err != nil {
			// 数字解析失败，忽略此行，不返回错误
			return nil, nil
		}

		// 解析请求路径
		path := p.extractPath(request)

		// 忽略状态码和body_bytes_sent，只关注bytes_sent
		_ = status
		_ = bodyBytesSentStr

		return &LogEntry{
			IP:        ip,
			BytesSent: bytesSent,
			Path:      path,
			Timestamp: timestamp,
		}, nil
	}

	// 如果正则匹配失败，尝试简单的空格分割方式
	// 标准nginx格式通常是: IP - - [timestamp] "request" status bytes_sent ...
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return nil, nil
	}

	var bytesSent int64
	var ip string
	var path string

	ip = fields[0]

	// 尝试找到请求路径（通常在引号内的字段）
	for i, field := range fields {
		if strings.HasPrefix(field, "\"") {
			// 合并引号内的所有字段
			request := field
			for j := i + 1; j < len(fields); j++ {
				request += " " + fields[j]
				if strings.HasSuffix(fields[j], "\"") {
					path = p.extractPath(request)
					break
				}
			}
			break
		}
	}

	// 从后往前找数字字段
	// 格式: ... status body_bytes_sent bytes_sent "referer" "user-agent"
	// bytes_sent是倒数第二个数字字段（在引号字符串之前）
	// 需要找到至少两个连续的数字字段才能确定 bytes_sent
	lastNonQuoteIndex := -1
	// 先找到最后一个非引号字段的索引
	for i := len(fields) - 1; i >= 0; i-- {
		if !strings.HasPrefix(fields[i], "\"") && !strings.HasSuffix(fields[i], "\"") {
			lastNonQuoteIndex = i
			break
		}
	}

	if lastNonQuoteIndex < 1 {
		return nil, nil
	}

	// 检查最后两个非引号字段是否都是数字（这应该是 body_bytes_sent 和 bytes_sent）
	// 如果倒数第二个非引号字段不是数字，说明格式不正确
	secondLastIndex := -1
	for i := lastNonQuoteIndex - 1; i >= 0; i-- {
		if !strings.HasPrefix(fields[i], "\"") && !strings.HasSuffix(fields[i], "\"") {
			secondLastIndex = i
			break
		}
	}

	if secondLastIndex < 0 {
		return nil, nil
	}

	// 检查这两个字段是否都是数字
	_, err1 := strconv.ParseInt(fields[lastNonQuoteIndex], 10, 64)
	secondLastVal, err2 := strconv.ParseInt(fields[secondLastIndex], 10, 64)

	// 如果倒数第二个字段不是数字，说明格式不正确，返回 nil
	if err2 != nil {
		return nil, nil
	}

	// 如果最后一个字段不是数字，也返回 nil（虽然理论上这可能是其他字段）
	if err1 != nil {
		return nil, nil
	}

	// bytes_sent 是倒数第二个数字字段
	bytesSent = secondLastVal

	return &LogEntry{
		IP:        ip,
		BytesSent: bytesSent,
		Path:      path,
		Timestamp: "",
	}, nil
}

// extractPath 从HTTP请求中提取路径（不包含查询参数）
func (p *LogParser) extractPath(request string) string {
	// 移除引号
	request = strings.Trim(request, "\"")

	// 格式通常是: "GET /path?query HTTP/1.1" 或 "POST /path HTTP/1.1"
	parts := strings.Fields(request)
	if len(parts) >= 2 {
		path := parts[1]
		// 去掉查询参数（?后面的部分）
		if idx := strings.IndexByte(path, '?'); idx >= 0 {
			path = path[:idx]
		}
		return path
	}

	// 如果只有路径部分
	if strings.HasPrefix(request, "/") {
		// 提取到第一个空格或问号
		if idx := strings.IndexAny(request, " ?"); idx > 0 {
			return request[:idx]
		}
		return request
	}

	return ""
}
