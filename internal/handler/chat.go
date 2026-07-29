package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"github.com/bytedance/gopkg/lang/fastrand"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/rechenz/TheDemiuge-Bridge/internal/agent"
	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
)

// ── 内存会话存储 ──

type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*agent.State
}

var store = &sessionStore{
	sessions: make(map[string]*agent.State),
}

func (s *sessionStore) getOrCreate(sessionID string, maxRounds int) *agent.State {
	// TODO: 先读锁查，如果存在就直接返回
	// TODO: 不存在就创建新 State，带 system prompt，返回
	s.mu.RLock()
	if state, ok := s.sessions[sessionID]; ok {
		s.mu.RUnlock()
		return state
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if state, ok := s.sessions[sessionID]; ok {
		return state
	}

	state := agent.NewState(maxRounds)
	s.sessions[sessionID] = state
	return state
}

// ── 请求 / 响应结构 ──

type chatRequest struct {
	SessionID string `json:"session_id"` // 空则新建会话
	Message   string `json:"message"`
	Stream    bool   `json:"stream"` // 是否流式返回
}

type chatResponse struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
}

// ── 路由处理器 ──

// Chat — POST /api/chat
func Chat(c context.Context, ctx *app.RequestContext) {
	cfg := config.Load()

	// 1. 解析请求体
	// TODO: ctx.BindJSON(&req)，失败返回 400
	var req chatRequest
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(400, utils.H{"error": "请求格式错误: " + err.Error()})
		return
	}
	// 2. 检查 message 不能为空
	if req.Message == "" {
		ctx.JSON(400, utils.H{"error": "message 不能为空"})
		return
	}
	// 3. 会话管理 — sessionID 为空则生成新的
	// TODO: sessionID = ... fastrand.Uint64()
	if req.SessionID == "" {
		req.SessionID = strconv.FormatUint(fastrand.Uint64(), 16)
	}
	// 4. 取/建 state
	// TODO: store.getOrCreate(sessionID, 10)
	state := store.getOrCreate(req.SessionID, 10)
	// 5. 检查轮次上限
	// TODO: state.Round >= state.MaxRounds → 返回 429
	if state.Round >= state.MaxRounds {
		ctx.JSON(429, utils.H{"error": "轮次超过上限"})
		return
	}
	// 6. 追加用户消息 + 轮次+1
	// TODO: append agent.Message{Role: "user", Content: req.Message}
	// TODO: state.Round++
	state.Messages = append(state.Messages, agent.Message{
		Role:    "user",
		Content: req.Message,
	})
	state.Round++
	// 7. 判断 stream 还是非 stream
	if req.Stream {
		// 流式：SSE
		// TODO: 设置 SSE 相关 header (Content-Type: text/event-stream, Cache-Control: no-cache, Connection: keep-alive)
		ctx.Response.Header.Set("Content-Type", "text/event-stream")
		ctx.Response.Header.Set("Cache-Control", "no-cache")
		ctx.Response.Header.Set("Connection", "keep-alive")
		ctx.Response.Header.Set("X-Accel-Buffering", "no")
		// TODO: 调 llm.ChatStream，在 onToken 里写 SSE data + flush
		// TODO: 返回完整回复后追加到 state.Messages
		// TODO: 写 [DONE] 结束标记
		fullReply, err := llm.ChatStream(c, cfg.DeepSeekKey, cfg.ModelName, state.Messages, cfg.MaxTokens, cfg.Temperature, func(token string) {
			data, _ := json.Marshal(chatResponse{
				SessionID: req.SessionID,
				Reply:     token,
			})
			ctx.Write([]byte("data: " + string(data) + "\n\n"))
			ctx.Flush()
		})

		if err != nil {
			errData, _ := json.Marshal(utils.H{"error": err.Error()})
			ctx.Write([]byte("data: " + string(errData) + "\n\n"))
			ctx.Flush()
			return
		}

		state.Messages = append(state.Messages, agent.Message{Role: "assistant", Content: fullReply})

		ctx.WriteString("data: [DONE]\n\n")
		ctx.Flush()

	} else {
		// 非流式：等完整结果返回 JSON
		// TODO: 调 llm.ChatStream, onToken 传 nil
		// TODO: 追加 assistant 回复到 state.Messages
		// TODO: ctx.JSON(200, chatResponse{...})
		fullReply, err := llm.ChatStream(c, cfg.DeepSeekKey, cfg.ModelName, state.Messages, cfg.MaxTokens, cfg.Temperature, nil)

		if err != nil {
			ctx.JSON(500, utils.H{"error": err.Error()})
			return
		}

		state.Messages = append(state.Messages, agent.Message{Role: "assistant", Content: fullReply})

		ctx.JSON(200, chatResponse{
			SessionID: req.SessionID,
			Reply:     fullReply,
		})

		ctx.Flush()
	}
}

// NewSession — POST /api/new-session
func NewSession(c context.Context, ctx *app.RequestContext) {
	// TODO: 生成新 sessionID, 创建 state, 返回 session_id
	sessionID := strconv.FormatUint(fastrand.Uint64(), 16)

	store.getOrCreate(sessionID, 10)

	ctx.JSON(200, utils.H{
		"session_id": sessionID,
	})
}

// Health — GET /health
func Health(c context.Context, ctx *app.RequestContext) {
	// TODO: 返回 {"status": "ok"}
	ctx.JSON(200, utils.H{"status": "ok"})
}
