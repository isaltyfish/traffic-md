package api

import (
	"net/http"
	"sort"
	"strconv"

	"traffic-md/internal/stats"

	"github.com/labstack/echo/v4"
)

// Tracker 接口，用于获取统计数据
type Tracker interface {
	GetStatsManager() *stats.StatsManager
}

// Server API 服务器
type Server struct {
	tracker Tracker
}

// NewServer 创建新的 API 服务器
func NewServer(tracker Tracker) *Server {
	return &Server{
		tracker: tracker,
	}
}

// IPStatResponse IP 统计响应
type IPStatResponse struct {
	IP        string  `json:"ip"`
	FirstSeen string  `json:"firstSeen"` // 格式: HH:mm:ss
	TrafficMB float64 `json:"trafficMB"` // 流量（MB）
}

// EndpointStatResponse 端点统计响应
type EndpointStatResponse struct {
	Endpoint  string  `json:"endpoint"`
	TrafficMB float64 `json:"trafficMB"` // 流量（MB）
}

// QPSResponse QPS统计响应
// TotalQPS: map[时间窗口]QPS值，如 {"5s": 100, "10s": 95, "1m": 80}
// EndpointQPS: map[端点]map[时间窗口]QPS值
type QPSResponse struct {
	TotalQPS    map[string]int64            `json:"totalQPS"`    // 总QPS（按时间窗口）
	EndpointQPS map[string]map[string]int64 `json:"endpointQPS"` // 端点QPS统计（按时间窗口）
}

// GetIPStats 获取 IP 统计
// GET /api/ip/{count}
func (s *Server) GetIPStats(c echo.Context) error {
	countStr := c.Param("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid count parameter, must be a positive integer",
		})
	}

	statsManager := s.tracker.GetStatsManager()
	ipStatsManager := statsManager.GetIPStats()
	allIPStats := ipStatsManager.GetAll()

	// 转换为响应格式并排序
	ipStatsList := make([]IPStatResponse, 0, len(allIPStats))
	for ip, stat := range allIPStats {
		firstSeen := stat.GetFirstSeen()
		firstSeenStr := ""
		if !firstSeen.IsZero() {
			firstSeenStr = firstSeen.Format("15:04:05")
		}
		ipStatsList = append(ipStatsList, IPStatResponse{
			IP:        ip,
			FirstSeen: firstSeenStr,
			TrafficMB: float64(stat.GetTotalBytes()) / (1024 * 1024), // 转换为 MB
		})
	}

	// 按流量从大到小排序
	sort.Slice(ipStatsList, func(i, j int) bool {
		return ipStatsList[i].TrafficMB > ipStatsList[j].TrafficMB
	})

	// 限制返回数量
	if count < len(ipStatsList) {
		ipStatsList = ipStatsList[:count]
	}

	return c.JSON(http.StatusOK, ipStatsList)
}

// GetEndpointStats 获取端点统计
// GET /api/endpoint/{count}
func (s *Server) GetEndpointStats(c echo.Context) error {
	countStr := c.Param("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid count parameter, must be a positive integer",
		})
	}

	statsManager := s.tracker.GetStatsManager()
	dailyStats := statsManager.GetCurrentDay()
	endpointStats := dailyStats.GetEndpointStats()

	// 转换为响应格式并排序
	endpointStatsList := make([]EndpointStatResponse, 0, len(endpointStats))
	for endpoint, bytes := range endpointStats {
		endpointStatsList = append(endpointStatsList, EndpointStatResponse{
			Endpoint:  endpoint,
			TrafficMB: float64(bytes) / (1024 * 1024), // 转换为 MB
		})
	}

	// 按流量从大到小排序
	sort.Slice(endpointStatsList, func(i, j int) bool {
		return endpointStatsList[i].TrafficMB > endpointStatsList[j].TrafficMB
	})

	// 限制返回数量
	if count < len(endpointStatsList) {
		endpointStatsList = endpointStatsList[:count]
	}

	return c.JSON(http.StatusOK, endpointStatsList)
}

// GetQPS 获取QPS统计
// GET /api/qps
func (s *Server) GetQPS(c echo.Context) error {
	statsManager := s.tracker.GetStatsManager()
	dailyStats := statsManager.GetCurrentDay()

	totalQPS := dailyStats.GetTotalQPS()
	endpointQPS := dailyStats.GetEndpointQPS()

	response := QPSResponse{
		TotalQPS:    totalQPS,
		EndpointQPS: endpointQPS,
	}

	return c.JSON(http.StatusOK, response)
}

// RegisterRoutes 注册路由
func (s *Server) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/ip/:count", s.GetIPStats)
	e.GET("/api/endpoint/:count", s.GetEndpointStats)
	e.GET("/api/qps", s.GetQPS)
}
