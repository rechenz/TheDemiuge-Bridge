package deepseek

import (
	"errors"
	"net/http"
	"testing"
)

// TestParseAPIErrorClassification 验证错误分类哨兵。
func TestParseAPIErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		kind   error
	}{
		{"auth", http.StatusUnauthorized, `{"error":{"message":"bad key","type":"auth","code":"invalid_api_key"}}`, ErrAuth},
		{"rate_limit", http.StatusTooManyRequests, `{"error":{"message":"too fast","type":"rate_limit","code":"429"}}`, ErrRateLimit},
		{"server", http.StatusInternalServerError, `{"error":{"message":"boom","type":"server_error","code":"500"}}`, ErrServer},
		{"unclassified", http.StatusBadRequest, `{"error":{"message":"bad request","type":"invalid_request","code":"400"}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseAPIError(tt.status, []byte(tt.body))
			if err == nil {
				t.Fatal("parseAPIError 返回 nil, want 错误")
			}
			if tt.kind == nil {
				return
			}
			if !errors.Is(err, tt.kind) {
				t.Errorf("errors.Is(err, %v) = false, err = %v", tt.kind, err)
			}
			var ce *ClassifiedError
			if !errors.As(err, &ce) {
				t.Fatalf("errors.As 无法解析为 *ClassifiedError")
			}
			if ce.API.Err.Message == "" {
				t.Error("API 详情丢失")
			}
		})
	}
}
