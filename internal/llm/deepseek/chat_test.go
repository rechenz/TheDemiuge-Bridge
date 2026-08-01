package deepseek

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// TestChatNonStream 验证非流式响应解析与工具定义透传。
func TestChatNonStream(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, `"tools"`) {
			t.Errorf("请求体未包含 tools: %s", bodyStr)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"今天天气晴朗"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))
	})

	tool := types.NewFunctionTool("get_weather", "查询天气", nil, false)
	opts := types.NewChatOptions([]types.Tool{*tool}, types.NewToolChoiceAuto())
	msgs := []types.Message{types.UserMessage{Content: "天气如何"}}

	agg, err := client.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}
	if got := *agg.Choices[0].Message.Content; got != "今天天气晴朗" {
		t.Errorf("Content = %q, want 今天天气晴朗", got)
	}
}
