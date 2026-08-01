package types

import (
	"encoding/json"
	"testing"
)

// TestMessageRoleSerialization 验证四类消息序列化时均显式携带 role 字段。
// 这是发往 LLM API 的对话请求正确性的基础——之前缺少 role 字段会导致
// DeepSeek 等 API 无法识别消息角色。
func TestMessageRoleSerialization(t *testing.T) {
	msgs := []Message{
		NewSystemMessage("你是 NPC"),
		NewUserMessage("你好"),
		AssistantMessage{Role: string(RoleAssistant), Content: strPtr("你好呀")},
		AssistantMessage{Role: string(RoleAssistant), ToolCalls: []ToolCall{{
			ID: "call_1", Type: ToolTypeFunction,
			Function: ToolCallFunction{Name: "play_animation", Arguments: `{"anim":"wave"}`},
		}}},
		NewToolMessage("动画播放成功", "call_1"),
	}

	wantRoles := []string{"system", "user", "assistant", "assistant", "tool"}
	for i, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal msgs[%d] 失败: %v", i, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal msgs[%d] 失败: %v", i, err)
		}
		role, ok := decoded["role"]
		if !ok {
			t.Errorf("msgs[%d](%s) 序列化缺少 role 字段,JSON: %s", i, msg.GetRole(), string(data))
			continue
		}
		if role != wantRoles[i] {
			t.Errorf("msgs[%d] role = %v,期望 %s,JSON: %s", i, role, wantRoles[i], string(data))
		}
	}
}

// TestRPCRequest_NoOmitOnNull 确保 assistant 仅携带 tool_calls 时 content 为 null。
func TestAssistantToolCallContentNull(t *testing.T) {
	msg := AssistantMessage{
		Role:      string(RoleAssistant),
		ToolCalls: []ToolCall{{ID: "c1", Type: ToolTypeFunction, Function: ToolCallFunction{Name: "f", Arguments: "{}"}}},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	content, ok := decoded["content"]
	if !ok {
		t.Fatalf("content 应为 null(显式输出),实际缺失,JSON: %s", string(data))
	}
	if content != nil {
		t.Errorf("content 应为 null,实际: %v", content)
	}
}

func strPtr(s string) *string { return &s }
