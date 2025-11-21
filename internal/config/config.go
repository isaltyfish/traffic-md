package config

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed app.yaml
var embeddedConfig embed.FS

// Config 应用配置
type Config struct {
	Web     WebConfig     `yaml:"web"`
	Monitor MonitorConfig `yaml:"monitor"`
	Log     LogConfig     `yaml:"log"`
	Report  ReportConfig  `yaml:"report"`
}

// WebConfig Web 服务器配置
type WebConfig struct {
	Port int `yaml:"port"`
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	Endpoints []string `yaml:"endpoints"`
}

// LogConfig 日志配置
type LogConfig struct {
	Path string `yaml:"path"`
}

// ReportConfig 报告配置
type ReportConfig struct {
	Interval string `yaml:"interval"`
}

// LoadConfig 从嵌入的配置文件加载配置
func LoadConfig() (*Config, error) {
	// 读取嵌入的配置文件
	data, err := embeddedConfig.ReadFile("app.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded config: %w", err)
	}

	// 解析 YAML 配置
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	// 设置默认值（如果配置文件中没有设置）
	if config.Web.Port == 0 {
		config.Web.Port = 8080
	}
	if config.Report.Interval == "" {
		config.Report.Interval = "5m"
	}
	if config.Monitor.Endpoints == nil {
		config.Monitor.Endpoints = []string{}
	}

	return &config, nil
}

// GetWebPort 获取 Web 端口
func (c *Config) GetWebPort() int {
	return c.Web.Port
}

// GetEndpoints 获取监控端点
func (c *Config) GetEndpoints() []string {
	return c.Monitor.Endpoints
}

// GetLogPath 获取日志路径
func (c *Config) GetLogPath() string {
	return c.Log.Path
}

// GetReportInterval 获取报告间隔
func (c *Config) GetReportInterval() string {
	return c.Report.Interval
}
