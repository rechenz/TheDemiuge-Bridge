// Package handler 提供 HTTP 业务处理器。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	hzss "github.com/cloudwego/hertz/pkg/protocol/sse"

	"github.com/rechenz/TheDemiuge-Bridge/internal/agent"
	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
	"github.com/rechenz/TheDemiuge-Bridge/internal/tool"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// ── 请求/事件协议 ──────────────────────────────────────────────────────────

// ChatRequest POST /api/chat 请求体。
type ChatRequest struct {
	// InstanceID UE5 实例 ID(agent 注册在该实例下)
	InstanceID string `json:"instance_id"`
	// Agent 要对话的 agent 名称(如 npc_alice)
	Agent string `json:"agent"`
	// SessionID 会话 ID(如玩家 ID)。同一会话连续对话共享上下文。
	SessionID string `json:"session_id"`
	// Message 玩家对 NPC 说的一句话
	Message string `json:"message"`
}

// SSE 事件类型常量(推送时逐字段透传,不做嵌套包装)。
const (
	// EventText 文本增量(delta)
	EventText = "text"
	// EventToolCall 工具调用(tool_call)
	EventToolCall = "tool_call"
	// EventCommentary 评述(推理/旁白,commentary)
	EventCommentary = "commentary"
	// EventDone 对话结束(done,含 reply/usage)
	EventDone = "done"
	// EventError 错误(error)
	EventError = "error"
)

// textEvent 文本增量事件。
type textEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

// toolCallEvent 工具调用事件。
type toolCallEvent struct {
	Type string        `json:"type"`
	Call types.ToolCall `json:"tool_call"`
}

// commentaryEvent 评述事件。
type commentaryEvent struct {
	Type       string           `json:"type"`
	Commentary agent.Commentary `json:"commentary"`
}

// doneEvent 对话结束事件。
type doneEvent struct {
	Type  string       `json:"type"`
	Reply string       `json:"reply"`
	Usage *types.Usage `json:"usage,omitempty"`
}

// errorEvent 错误事件。
type errorEvent struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// ── ChatHandler ─────────────────────────────────────────────────────────────

// ChatHandler 处理 NPC 对话入口。
// 每个 (instance, agent) 对缓存一个 Runner(内含 Agent 会话上下文),
// 同一 agent 的多次请求共享历史;不同会话由 SessionID 隔离。
type ChatHandler struct {
	mu       sync.Mutex
	runners  map[string]*chatEntry // key: instanceID + "/" + agentName
	mgr      *ue5.Manager
	cli      *ue5.Client
	provider llm.Provider
	debug    bool
}

// chatEntry 一个 agent 的 Runner 及其并发锁。
// Runner 本身不保证并发安全,同 agent 的请求串行执行。
type chatEntry struct {
	mu     sync.Mutex
	runner *agent.Runner
}

// NewChatHandler 构造对话处理器。
func NewChatHandler(mgr *ue5.Manager, cli *ue5.Client, provider llm.Provider, debug bool) *ChatHandler {
	return &ChatHandler{
		runners:  make(map[string]*chatEntry),
		mgr:      mgr,
		cli:      cli,
		provider: provider,
		debug:    debug,
	}
}

// Chat POST /api/chat 处理函数(SSE 流式)。
func (h *ChatHandler) Chat(ctx context.Context, c *app.RequestContext) {
	var req ChatRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, errorEvent{Type: EventError, Error: "请求体非法: " + err.Error()})
		return
	}
	if req.InstanceID == "" || req.Agent == "" || req.Message == "" {
		c.JSON(consts.StatusBadRequest, errorEvent{Type: EventError, Error: "instance_id / agent / message 不能为空"})
		return
	}
	if req.SessionID == "" {
		req.SessionID = "default"
	}

	entry, err := h.getOrCreateRunner(req.InstanceID, req.Agent)
	if err != nil {
		c.JSON(consts.StatusBadRequest, errorEvent{Type: EventError, Error: err.Error()})
		return
	}

	// 建立 SSE 流
	writer := hzss.NewWriter(c)
	if err := writer.WriteEvent("", "connected", []byte("connected")); err != nil {
		return
	}
	sink := &chatSink{writer: writer}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	result, err := entry.runner.Run(ctx, req.SessionID, req.Message, sink)
	if err != nil {
		_ = writer.WriteEvent("", EventError, mustJSON(errorEvent{Type: EventError, Error: err.Error()}))
		return
	}

	_ = writer.WriteEvent("", EventDone, mustJSON(doneEvent{Type: EventDone, Reply: result.Reply, Usage: result.Usage}))
}

// getOrCreateRunner 按 (instance, agent) 取缓存 Runner,不存在时构建。
func (h *ChatHandler) getOrCreateRunner(instanceID, agentName string) (*chatEntry, error) {
	key := instanceID + "/" + agentName

	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.runners[key]; ok {
		return e, nil
	}

	// 构建 Agent(从实例注册空间取定义 + 工具)
	agentDef, ok := h.mgr.GetAgent(instanceID, agentName)
	if !ok {
		return nil, fmt.Errorf("agent %q 未在实例 %q 中注册", agentName, instanceID)
	}
	inst, ok := h.mgr.GetInstance(instanceID)
	if !ok {
		return nil, fmt.Errorf("实例 %q 不存在", instanceID)
	}

	a := types.NewAgent(agentDef.Name, types.AgentType(agentDef.Type))
	a.SetSystemPrompt(agentDef.SystemPrompt)

	var tools []types.Tool
	for _, name := range agentDef.Tools {
		if reg, ok := h.mgr.GetTool(instanceID, name); ok {
			tools = append(tools, reg.ToTool())
		}
	}
	a.SetTools(tools...)

	executor := tool.NewUE5Executor(h.mgr, h.cli, inst)
	runner := agent.NewRunner(a, h.provider, executor, agent.WithDebug(h.debug))

	e := &chatEntry{runner: runner}
	h.runners[key] = e
	return e, nil
}

// ── EventSink 实现 ──────────────────────────────────────────────────────────

// chatSink 把 Agent 运行事件写入 SSE 流。
type chatSink struct {
	writer *hzss.Writer
}

// OnText 实时推送文本增量。
func (s *chatSink) OnText(delta string) error {
	return s.writer.WriteEvent("", EventText, mustJSON(textEvent{Type: EventText, Delta: delta}))
}

// OnToolCall 推送一次工具调用事件。
func (s *chatSink) OnToolCall(call types.ToolCall) error {
	return s.writer.WriteEvent("", EventToolCall, mustJSON(toolCallEvent{Type: EventToolCall, Call: call}))
}

// OnCommentary 推送评述(默认透传;前端不需要可忽略)。
func (s *chatSink) OnCommentary(c agent.Commentary) error {
	return s.writer.WriteEvent("", EventCommentary, mustJSON(commentaryEvent{Type: EventCommentary, Commentary: c}))
}

// OnDebug 调试信息,同通道推送(客户端可选择性忽略)。
func (s *chatSink) OnDebug(msg string) error {
	return s.writer.WriteEvent("", "debug", []byte(msg))
}

// mustJSON 序列化事件对象;失败时返回空对象(不应发生,字段均为基础类型)。
func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// ── 鉴权 ────────────────────────────────────────────────────────────────────

// AuthMiddleware 校验 X-API-Key。
// 配置为空时不鉴权(本地联调);校验失败返回 401。
func (h *ChatHandler) AuthMiddleware(cfg *config.Config) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if cfg.ChatAPIKey != "" && c.Request.Header.Get("X-API-Key") != cfg.ChatAPIKey {
			c.JSON(consts.StatusUnauthorized, errorEvent{Type: EventError, Error: "X-API-Key 不匹配"})
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
