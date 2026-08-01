package agent

import (
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// helper:构造 n 条 user 消息历史。
func userHistory(n int) []types.Message {
	out := make([]types.Message, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, types.NewUserMessage("hello"))
	}
	return out
}

// TestTrimHistoryWindow_UnderLimit 验证未超限时原样返回。
func TestTrimHistoryWindow_UnderLimit(t *testing.T) {
	h := userHistory(5)
	got := trimHistoryWindow(h)
	if len(got) != 5 {
		t.Fatalf("裁剪后长度 = %d,期望 5", len(got))
	}
}

// TestTrimHistoryWindow_TrimsOldest 验证超限时保留最近的消息,丢弃最旧的。
func TestTrimHistoryWindow_TrimsOldest(t *testing.T) {
	h := userHistory(maxHistoryMessages + 10)
	got := trimHistoryWindow(h)
	if len(got) != maxHistoryMessages {
		t.Fatalf("裁剪后长度 = %d,期望 %d", len(got), maxHistoryMessages)
	}
	// 裁剪后第一条应是最新的 user 消息(角色正确即可)
	if got[0].GetRole() != string(types.RoleUser) {
		t.Errorf("裁剪后首条角色 = %q,期望 user", got[0].GetRole())
	}
}

// TestTrimHistoryWindow_KeepsToolPair 验证裁剪时不会把 tool 消息与其
// assistant(tool_calls) 配对拆散,且首条不会是无配对的 tool 消息。
func TestTrimHistoryWindow_KeepsToolPair(t *testing.T) {
	// 构造 40 条历史:user, assistant(tool_calls), tool, user, ...
	// 总长 40 > 30,触发裁剪。
	var h []types.Message
	for i := 0; i < 10; i++ {
		h = append(h, types.NewUserMessage("ask"))
		h = append(h, types.AssistantMessage{
			Role: string(types.RoleAssistant),
			ToolCalls: []types.ToolCall{{
				ID:   "call",
				Type: "function",
				Function: types.ToolCallFunction{
					Name:      "tool",
					Arguments: `{}`,
				},
			}},
		})
		h = append(h, types.NewToolMessage("result", "call"))
	}
	// 末尾补一个普通 user(模拟最新提问被附加后)
	h = append(h, types.NewUserMessage("latest"))

	got := trimHistoryWindow(h)
	if len(got) > maxHistoryMessages {
		t.Fatalf("裁剪后长度 = %d,超出上限 %d", len(got), maxHistoryMessages)
	}
	// 首条不允许是 tool 消息(必须与其 assistant 配对)
	if got[0].GetRole() == string(types.RoleTool) {
		t.Error("裁剪后首条不应是无配对的 tool 消息")
	}
	// 末条应保留最新的 user 消息
	if got[len(got)-1].GetContent() != "latest" {
		t.Errorf("末条内容 = %q,期望 latest", got[len(got)-1].GetContent())
	}
}
