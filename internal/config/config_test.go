package config

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证默认值
	if cfg.GetWebPort() == 0 {
		t.Error("WebPort should not be 0")
	}

	// 验证配置是否被正确读取
	// 根据 app.yaml，默认端口应该是 8080
	if cfg.GetWebPort() != 8080 {
		t.Errorf("Expected WebPort to be 8080, got %d", cfg.GetWebPort())
	}

	// 验证报告间隔
	if cfg.GetReportInterval() == "" {
		t.Error("ReportInterval should not be empty")
	}

	// 验证报告间隔格式
	if cfg.GetReportInterval() != "5m" {
		t.Errorf("Expected ReportInterval to be '5m', got '%s'", cfg.GetReportInterval())
	}

	// 验证端点配置（根据 app.yaml，应该包含 /api/** 和 /static/**）
	endpoints := cfg.GetEndpoints()
	if len(endpoints) == 0 {
		t.Error("Endpoints should not be empty (should contain /api/** and /static/**)")
	}

	// 验证端点内容
	expectedEndpoints := []string{"/api/**", "/static/**"}
	if len(endpoints) != len(expectedEndpoints) {
		t.Errorf("Expected %d endpoints, got %d", len(expectedEndpoints), len(endpoints))
	}
	for i, expected := range expectedEndpoints {
		if i < len(endpoints) && endpoints[i] != expected {
			t.Errorf("Expected endpoint[%d] to be '%s', got '%s'", i, expected, endpoints[i])
		}
	}
}
