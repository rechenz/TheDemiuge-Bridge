package deepseek

import (
	"net/http"
	"testing"
	"time"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
)

// TestNewClient_TimeoutFallback 验证未配置任何超时与注入客户端时,
// 兜底超时生效(HTTP_TIMEOUT=0 场景),请求不会永久挂起。
func TestNewClient_TimeoutFallback(t *testing.T) {
	cfg := &config.Config{
		APIKey:      "test-key",
		BaseURL:     "http://127.0.0.1:1", // 不可达地址
		ModelName:   "test-model",
		HTTPClient:  nil, // 无注入客户端
		HTTPTimeout: 0,   // 无显式超时 → 触发兜底
	}
	client := NewClient(cfg)
	if client.httpClient == nil {
		t.Fatal("NewClient 不应产生 nil httpClient")
	}
	if client.httpClient.Timeout != defaultHTTPTimeout {
		t.Errorf("兜底超时 = %v,期望 %v", client.httpClient.Timeout, defaultHTTPTimeout)
	}
}

// TestNewClient_RespectsConfiguredTimeout 验证显式配置超时时按配置走。
func TestNewClient_RespectsConfiguredTimeout(t *testing.T) {
	cfg := &config.Config{
		HTTPClient:  nil,
		HTTPTimeout: 30 * time.Second,
	}
	client := NewClient(cfg)
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("超时 = %v,期望 30s", client.httpClient.Timeout)
	}
}

// TestNewClient_InjectedClientWins 验证注入的 http.Client 优先于默认构造。
func TestNewClient_InjectedClientWins(t *testing.T) {
	injected := &http.Client{Timeout: 5 * time.Second}
	cfg := &config.Config{
		HTTPClient:  injected,
		HTTPTimeout: 0,
	}
	client := NewClient(cfg)
	if client.httpClient != injected {
		t.Error("注入的 http.Client 应被直接使用")
	}
}
