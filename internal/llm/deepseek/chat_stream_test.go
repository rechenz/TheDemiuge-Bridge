package deepseek

import (
	"context"
	"net/http"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// TestChatStreamToolCallsAndFinishReason 验证:
//  1. 流式 tool_calls 增量按 index 拼接为完整调用;
//  2. usage 聚合到最终响应;
//  3. finish_reason = tool_calls 被保留(而非硬编码 stop)。
func TestChatStreamToolCallsAndFinishReason(t *testing.T) {
	sse := "data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"上海\"}"}}]},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}` + "\n\n" +
		"data: [DONE]\n\n"

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want Bearer test-key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	})

	msgs := []types.Message{types.UserMessage{Content: "上海天气如何?"}}
	var chunks []*types.ChatCompletionStreamChunk
	agg, err := client.ChatStream(context.Background(), msgs, nil, func(c *types.ChatCompletionStreamChunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}

	if got := len(chunks); got != 4 {
		t.Fatalf("onChunk 回调次数 = %d, want 4(含 usage chunk)", got)
	}
	if agg.Choices[0].FinishReason != types.FinishReasonToolCalls {
		t.Errorf("FinishReason = %q, want %q", agg.Choices[0].FinishReason, types.FinishReasonToolCalls)
	}
	if len(agg.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls 数量 = %d, want 1", len(agg.Choices[0].Message.ToolCalls))
	}
	tc := agg.Choices[0].Message.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "get_weather" {
		t.Errorf("ToolCall = %+v, want id=call_1 name=get_weather", tc)
	}
	if tc.Function.Arguments != `{"city":"上海"}` {
		t.Errorf("Arguments = %q, want 完整 JSON 拼接", tc.Function.Arguments)
	}
	if agg.Usage.TotalTokens != 15 {
		t.Errorf("Usage.TotalTokens = %d, want 15", agg.Usage.TotalTokens)
	}
}

// TestChatStreamTextAndReasoning 验证文本与推理内容聚合,finish_reason=stop。
func TestChatStreamTextAndReasoning(t *testing.T) {
	sse := "data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"先想想"},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"你好"},"finish_reason":null}]}` + "\n\n" +
		"data: " + `{"id":"x","object":"chat.completion.chunk","created":1,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"世界"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	})

	msgs := []types.Message{types.UserMessage{Content: "hi"}}
	agg, err := client.ChatStream(context.Background(), msgs, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream error: %v", err)
	}
	msg := agg.Choices[0].Message
	if got := *msg.Content; got != "你好世界" {
		t.Errorf("Content = %q, want 你好世界", got)
	}
	if got := *msg.ReasoningContent; got != "先想想" {
		t.Errorf("ReasoningContent = %q, want 先想想", got)
	}
	if agg.Choices[0].FinishReason != types.FinishReasonStop {
		t.Errorf("FinishReason = %q, want stop", agg.Choices[0].FinishReason)
	}
}
