package deepseek

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// newTestClient 构造指向 httptest 服务端的 Client。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &config.Config{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		ModelName:  types.ModelV4Flash,
		HTTPClient: srv.Client(),
	}
	return NewClient(cfg)
}
