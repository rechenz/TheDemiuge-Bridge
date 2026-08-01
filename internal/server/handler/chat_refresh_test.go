package handler

import (
	"context"
	"sync"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// capturingProvider 记录每次 Chat 调用收到的消息与工具选项,供断言动态刷新。
type capturingProvider struct {
	mu       sync.Mutex
	lastOpts *types.ChatOptions
}

func (p *capturingProvider) Chat(_ context.Context, _ []types.Message, opts *types.ChatOptions) (*types.ChatResponse, error) {
	p.mu.Lock()
	p.lastOpts = opts
	p.mu.Unlock()
	return &types.ChatResponse{
		Choices: []types.ChatChoice{{
			Message:      types.NewAssistantMessage("你好"),
			FinishReason: types.FinishReasonStop,
		}},
	}, nil
}

func (p *capturingProvider) ChatStream(_ context.Context, _ []types.Message, opts *types.ChatOptions, _ func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {
	return p.Chat(context.Background(), nil, opts)
}

func (p *capturingProvider) toolNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastOpts == nil {
		return nil
	}
	out := make([]string, 0, len(p.lastOpts.Tools))
	for _, t := range p.lastOpts.Tools {
		out = append(out, t.Function.Name)
	}
	return out
}

// toolReg 便捷构造一个 ToolReg。
func toolReg(t *testing.T, name, desc string) ue5.ToolReg {
	t.Helper()
	return ue5.ToolReg{
		ToolDef: mustToolDef(t, `{
			"name": "`+name+`",
			"description": "`+desc+`"
		}`),
	}
}

// TestChatHandler_ToolHotReload 验证 Runner 缓存下,UE5 侧热更新工具列表后,
// 下一次 /api/chat 请求传给 LLM 的工具定义是实时的:
//  1. 首次请求:LLM 收到 tool_a;
//  2. 热更新:agent 改为引用 tool_b(删除 tool_a);
//  3. 再次请求:LLM 只收到 tool_b,删掉的 tool_a 不再出现。
func TestChatHandler_ToolHotReload(t *testing.T) {
	mgr := ue5.NewManager(ue5.WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", "")
	if err := mgr.UpsertTool(inst.ID, toolReg(t, "tool_a", "工具A")); err != nil {
		t.Fatalf("注册 tool_a 失败: %v", err)
	}
	if err := mgr.UpsertTool(inst.ID, toolReg(t, "tool_b", "工具B")); err != nil {
		t.Fatalf("注册 tool_b 失败: %v", err)
	}
	if err := mgr.UpsertAgent(inst.ID, ue5.AgentDef{
		Name: "npc", Type: "actor", SystemPrompt: "旧提示词", Tools: []string{"tool_a"},
	}); err != nil {
		t.Fatalf("注册 agent 失败: %v", err)
	}

	provider := &capturingProvider{}
	adapter := ue5.NewRegistryAdapter(mgr, nil)
	h := NewChatHandler(adapter, provider, false)

	// 1. 创建 Runner 缓存
	entry, err := h.getOrCreateRunner(inst.ID, "npc")
	if err != nil {
		t.Fatalf("getOrCreateRunner 失败: %v", err)
	}

	// 2. 热更新:system prompt 变化 + 工具引用从 tool_a 改为 tool_b
	if err := mgr.UpsertAgent(inst.ID, ue5.AgentDef{
		Name: "npc", Type: "actor", SystemPrompt: "新提示词", Tools: []string{"tool_b"},
	}); err != nil {
		t.Fatalf("热更新 agent 失败: %v", err)
	}

	// 3. 模拟 Chat 请求:先 refreshRunner 同步最新定义,再运行 ReAct
	entry.mu.Lock()
	if err := h.refreshRunner(entry); err != nil {
		entry.mu.Unlock()
		t.Fatalf("refreshRunner 失败: %v", err)
	}
	_, err = entry.runner.Run(context.Background(), "player_1", "你好", nil)
	entry.mu.Unlock()
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}

	// 4. 断言 LLM 收到的工具已动态更新
	names := provider.toolNames()
	if len(names) != 1 || names[0] != "tool_b" {
		t.Fatalf("LLM 收到的工具 = %v,期望 [tool_b]", names)
	}
}

// TestChatHandler_PromptHotReload 验证热更新 system prompt 后 Runner 使用新 prompt。
func TestChatHandler_PromptHotReload(t *testing.T) {
	mgr := ue5.NewManager(ue5.WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", "")
	if err := mgr.UpsertAgent(inst.ID, ue5.AgentDef{
		Name: "npc", Type: "actor", SystemPrompt: "旧提示词",
	}); err != nil {
		t.Fatalf("注册 agent 失败: %v", err)
	}

	h := NewChatHandler(ue5.NewRegistryAdapter(mgr, nil), &capturingProvider{}, false)
	entry, err := h.getOrCreateRunner(inst.ID, "npc")
	if err != nil {
		t.Fatalf("getOrCreateRunner 失败: %v", err)
	}

	// 热更新 system prompt
	if err := mgr.UpsertAgent(inst.ID, ue5.AgentDef{
		Name: "npc", Type: "actor", SystemPrompt: "新提示词",
	}); err != nil {
		t.Fatalf("热更新 agent 失败: %v", err)
	}

	// refreshRunner 后应使用新 prompt
	if err := h.refreshRunner(entry); err != nil {
		t.Fatalf("refreshRunner 失败: %v", err)
	}
	if got := entry.runner.GetAgent().SystemPrompt; got != "新提示词" {
		t.Errorf("system prompt = %q,期望 新提示词", got)
	}

	// agent 删除后 refreshRunner 应报错
	if !mgr.DeleteAgent(inst.ID, "npc") {
		t.Fatal("DeleteAgent 失败")
	}
	if err := h.refreshRunner(entry); err == nil {
		t.Error("agent 删除后 refreshRunner 应返回错误")
	}
}

var _ llm.Provider = (*capturingProvider)(nil)
