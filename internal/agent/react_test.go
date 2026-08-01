package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ── mock Provider ───────────────────────────────────────────────────────────

// scriptedProvider 按序返回预设响应,模拟 LLM 行为。
// 用于测试 ReAct 循环的工具调用轮与最终回复轮。
type scriptedProvider struct {
	responses []*types.ChatResponse
	idx       int
}

func (p *scriptedProvider) Chat(ctx context.Context, messages []types.Message, opts *types.ChatOptions) (*types.ChatResponse, error) {
	if p.idx >= len(p.responses) {
		return nil, errors.New("mock provider 响应用尽")
	}
	resp := p.responses[p.idx]
	p.idx++
	return resp, nil
}

func (p *scriptedProvider) ChatStream(ctx context.Context, messages []types.Message, opts *types.ChatOptions, onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {
	return p.Chat(ctx, messages, opts)
}

// toolCallsResp 构造一个仅包含工具调用的响应。
func toolCallsResp(calls ...types.ToolCall) *types.ChatResponse {
	return &types.ChatResponse{
		Choices: []types.ChatChoice{{
			Message:      types.AssistantMessage{ToolCalls: calls},
			FinishReason: types.FinishReasonToolCalls,
		}},
		Usage: types.Usage{TotalTokens: 10},
	}
}

// textResp 构造一个最终文本回复响应。
func textResp(content string) *types.ChatResponse {
	return &types.ChatResponse{
		Choices: []types.ChatChoice{{
			Message:      types.NewAssistantMessage(content),
			FinishReason: types.FinishReasonStop,
		}},
		Usage: types.Usage{TotalTokens: 20},
	}
}

// ── mock ToolExecutor ───────────────────────────────────────────────────────

// fakeExecutor 记录收到的调用并返回预设结果。
type fakeExecutor struct {
	calls   []types.ToolCall
	results map[string]string // name -> result
}

func (e *fakeExecutor) Execute(ctx context.Context, call types.ToolCall) (string, error) {
	e.calls = append(e.calls, call)
	if r, ok := e.results[call.Function.Name]; ok {
		return r, nil
	}
	return "ok", nil
}

// ── 测试 ────────────────────────────────────────────────────────────────────

func TestRunner_ToolCallLoop(t *testing.T) {
	agent := types.NewAgent("npc", types.AgentTypeActor,
		types.WithSystemPrompt("你是 NPC"),
		types.WithTools(*types.NewFunctionTool("play_animation", "播放动画", nil, false)),
	)

	provider := &scriptedProvider{responses: []*types.ChatResponse{
		toolCallsResp(types.ToolCall{
			ID:   "call_1",
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      "play_animation",
				Arguments: `{"anim":"wave"}`,
			},
		}),
		textResp("我向你挥了挥手。"),
	}}

	exec := &fakeExecutor{results: map[string]string{"play_animation": "动画播放成功"}}
	runner := NewRunner(agent, provider, exec)

	sink := &recordingSink{}
	result, err := runner.Run(context.Background(), "session_1", "你好", sink)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 工具应被执行一次
	if len(exec.calls) != 1 {
		t.Fatalf("工具执行次数 = %d,期望 1", len(exec.calls))
	}
	if exec.calls[0].Function.Name != "play_animation" {
		t.Errorf("调用的工具 = %q,期望 play_animation", exec.calls[0].Function.Name)
	}

	// 最终回复
	if result.Reply != "我向你挥了挥手。" {
		t.Errorf("最终回复 = %q", result.Reply)
	}
	if result.Usage == nil || result.Usage.TotalTokens != 20 {
		t.Errorf("usage 应为最后一次响应,得到 %+v", result.Usage)
	}

	// 会话历史应包含: user → assistant(tool_calls) → tool → assistant(回复)
	msgs := agent.GetMessages("session_1")
	if len(msgs) != 4 {
		t.Fatalf("会话历史长度 = %d,期望 4", len(msgs))
	}
	if msgs[2].GetRole() != string(types.RoleTool) {
		t.Errorf("第 3 条消息角色 = %q,期望 tool", msgs[2].GetRole())
	}
	if msgs[2].GetContent() != "动画播放成功" {
		t.Errorf("工具结果 = %q,期望 动画播放成功", msgs[2].GetContent())
	}
}

func TestRunner_SingleTurn(t *testing.T) {
	agent := types.NewAgent("npc", types.AgentTypeActor)
	provider := &scriptedProvider{responses: []*types.ChatResponse{textResp("你好呀")}}
	runner := NewRunner(agent, provider, nil)

	result, err := runner.Run(context.Background(), "s1", "在吗", nil)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if result.Reply != "你好呀" {
		t.Errorf("回复 = %q,期望 你好呀", result.Reply)
	}
}

func TestRunner_MaxRounds(t *testing.T) {
	agent := types.NewAgent("npc", types.AgentTypeActor,
		types.WithTools(*types.NewFunctionTool("loop", "循环", nil, false)),
	)

	// 一直返回工具调用,触发最大轮次保护
	provider := &scriptedProvider{responses: []*types.ChatResponse{
		toolCallsResp(types.ToolCall{ID: "c1", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c2", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c3", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c4", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c5", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c6", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c7", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c8", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c9", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c10", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
		toolCallsResp(types.ToolCall{ID: "c11", Type: "function", Function: types.ToolCallFunction{Name: "loop", Arguments: `{}`}}),
	}}
	runner := NewRunner(agent, provider, &fakeExecutor{}, WithMaxRounds(10))

	_, err := runner.Run(context.Background(), "s1", "开始", nil)
	if err == nil {
		t.Fatal("工具无限循环应触发最大轮次错误")
	}
}

func TestRunner_NoExecutor(t *testing.T) {
	agent := types.NewAgent("npc", types.AgentTypeActor,
		types.WithTools(*types.NewFunctionTool("some_tool", "工具", nil, false)),
	)
	provider := &scriptedProvider{responses: []*types.ChatResponse{
		toolCallsResp(types.ToolCall{ID: "c1", Type: "function", Function: types.ToolCallFunction{Name: "some_tool", Arguments: `{}`}}),
	}}
	runner := NewRunner(agent, provider, nil)

	_, err := runner.Run(context.Background(), "s1", "hi", nil)
	if err == nil {
		t.Fatal("未配置 executor 时应返回错误")
	}
}

// ── recordingSink ───────────────────────────────────────────────────────────

// recordingSink 记录推送的文本增量,供断言。
type recordingSink struct {
	texts []string
}

func (s *recordingSink) OnText(delta string) error {
	s.texts = append(s.texts, delta)
	return nil
}
func (s *recordingSink) OnToolCall(call types.ToolCall) error { return nil }
func (s *recordingSink) OnCommentary(c Commentary) error      { return nil }
func (s *recordingSink) OnDebug(msg string) error             { return nil }
