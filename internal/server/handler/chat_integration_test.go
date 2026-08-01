package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/backend"
)

// ── mock Provider(双轮:工具调用 → 最终回复)───────────────────────────────

// mockProvider 模拟 LLM:第一轮返回工具调用,第二轮返回最终文本。
type mockProvider struct {
	round int
}

func (p *mockProvider) Chat(ctx context.Context, messages []types.Message, opts *types.ChatOptions) (*types.ChatResponse, error) {
	p.round++
	if p.round == 1 {
		return &types.ChatResponse{
			Choices: []types.ChatChoice{{
				Message: types.AssistantMessage{ToolCalls: []types.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: types.ToolCallFunction{
						Name:      "play_animation",
						Arguments: `{"anim":"wave"}`,
					},
				}}},
				FinishReason: types.FinishReasonToolCalls,
			}},
			Usage: types.Usage{TotalTokens: 10},
		}, nil
	}
	return &types.ChatResponse{
		Choices: []types.ChatChoice{{
			Message:      types.NewAssistantMessage("我向你挥了挥手。"),
			FinishReason: types.FinishReasonStop,
		}},
		Usage: types.Usage{TotalTokens: 20},
	}, nil
}

func (p *mockProvider) ChatStream(ctx context.Context, messages []types.Message, opts *types.ChatOptions, onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {
	return p.Chat(ctx, messages, opts)
}

// ── 集成测试:POST /api/chat 全链路 ────────────────────────────────────────

// TestChatIntegration 起真实 Hertz 服务 + mock UE5 + mock LLM,
// 验证:注册实例/agent/tool → /api/chat SSE 流 → ReAct 工具调用 → 最终回复。
func TestChatIntegration(t *testing.T) {
	// 1. mock UE5 工具执行端
	ue5Calls := 0
	ue5Srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ue5Calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"anim":"wave","played":true}}`))
	}))
	defer ue5Srv.Close()

	// 2. 注册空间:实例 + 工具 + agent
	mgr := backend.NewManager(backend.WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", ue5Srv.URL)
	if err := mgr.UpsertTool(inst.ID, backend.ToolReg{
		ToolDef: mustToolDef(t, `{
			"name": "play_animation",
			"description": "让 NPC 播放指定动画",
			"parameters": {
				"type": "object",
				"properties": {"anim": {"type": "string", "description": "动画名"}},
				"required": ["anim"]
			}
		}`),
	}); err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}
	if err := mgr.UpsertAgent(inst.ID, backend.AgentDef{
		Name:         "npc_alice",
		Type:         "actor",
		SystemPrompt: "你是面包店老板娘艾丽丝,温柔友善。",
		Tools:        []string{"play_animation"},
	}); err != nil {
		t.Fatalf("注册 agent 失败: %v", err)
	}

	// 3. 起 Hertz 服务
	cli := &backend.Client{HTTPClient: ue5Srv.Client()}
	chatHandler := NewChatHandler(backend.NewRegistryAdapter(mgr, cli), &mockProvider{}, false)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听端口失败: %v", err)
	}
	addr := ln.Addr().String()
	h := server.Default(server.WithListener(ln))
	h.POST("/api/chat", chatHandler.Chat)
	go h.Spin()
	defer h.Close()
	waitEngineReady(t, addr)

	// 4. 调用 /api/chat
	body, _ := json.Marshal(ChatRequest{
		InstanceID: "inst_a",
		Agent:      "npc_alice",
		SessionID:  "player_1",
		Message:    "早上好",
	})
	resp, err := http.Post("http://"+addr+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("调用 /api/chat 失败: %v", err)
	}
	defer resp.Body.Close()

	// 5. 逐行读 SSE,收集事件
	scanner := bufio.NewScanner(resp.Body)
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			events = append(events, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("读 SSE 流失败: %v", err)
	}

	// 6. 断言事件序列
	joined := strings.Join(events, "|")
	for _, want := range []string{"connected", "tool_call", "done"} {
		if !strings.Contains(joined, want) {
			t.Errorf("SSE 流缺少事件 %q,实际: %s", want, joined)
		}
	}
	if !strings.Contains(joined, `"reply":"我向你挥了挥手。"`) {
		t.Errorf("done 事件缺少最终回复,实际: %s", joined)
	}

	// 7. UE5 工具应被真实调用一次
	if ue5Calls != 1 {
		t.Errorf("UE5 工具调用次数 = %d,期望 1", ue5Calls)
	}
}

// ── 工具 ────────────────────────────────────────────────────────────────────

// mustToolDef 从 JSON 构造 backend.ToolDef(toolParams 为私有类型,只能走反序列化)。
func mustToolDef(t *testing.T, raw string) backend.ToolDef {
	t.Helper()
	var def backend.ToolDef
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		t.Fatalf("构造 ToolDef 失败: %v", err)
	}
	return def
}

// waitEngineReady 轮询等待服务可访问。
func waitEngineReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("服务启动超时")
}
