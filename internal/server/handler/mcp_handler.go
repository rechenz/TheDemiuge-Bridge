package handler

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	hzss "github.com/cloudwego/hertz/pkg/protocol/sse"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/rechenz/TheDemiuge-Bridge/internal/mcp"
)

// MCPHandler 处理 MCP 协议入口。
// POST /mcp/{instance_id}   —— JSON-RPC 单次请求/批量请求,返回 JSON 响应
// GET  /mcp/{instance_id}   —— SSE 长连接,推送该实例的工具/agent 变更通知
type MCPHandler struct {
	server *mcp.Server
	hub    *mcp.Hub
}

// NewMCPHandler 构造 MCP 处理器。
func NewMCPHandler(server *mcp.Server, hub *mcp.Hub) *MCPHandler {
	return &MCPHandler{server: server, hub: hub}
}

// RegisterRoutes 注册 MCP 路由。
// 挂载在 /mcp 下(由调用方传入路由组或直接挂 Hertz)。
func (h *MCPHandler) RegisterRoutes(group *route.RouterGroup) {
	group.POST("/:instance_id", h.post)
	group.GET("/:instance_id", h.sse)
}

// post 处理 JSON-RPC 请求(POST,单发/批量)。
func (h *MCPHandler) post(ctx context.Context, c *app.RequestContext) {
	instanceID := c.Param("instance_id")
	body := c.Request.Body()

	if len(body) == 0 {
		c.JSON(consts.StatusBadRequest, mcp.NewErrorResponse("", mcp.CodeInvalidRequest, "请求体为空"))
		return
	}

	// 批量请求:body 以 '[' 开头 → 数组形式
	if len(body) > 0 && body[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(body, &batch); err != nil {
			c.JSON(consts.StatusBadRequest, mcp.NewErrorResponse("", mcp.CodeParseError, "JSON 解析失败: "+err.Error()))
			return
		}
		responses := make([]*mcp.Response, 0, len(batch))
		for _, raw := range batch {
			var req mcp.Request
			if err := json.Unmarshal(raw, &req); err != nil {
				responses = append(responses, mcp.NewErrorResponse("", mcp.CodeParseError, "JSON 解析失败: "+err.Error()))
				continue
			}
			if resp := h.server.Handle(ctx, instanceID, &req); resp != nil {
				responses = append(responses, resp)
			}
		}
		if len(responses) == 0 {
			c.Status(consts.StatusNoContent)
			return
		}
		c.JSON(consts.StatusOK, responses)
		return
	}

	// 单条请求
	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(consts.StatusBadRequest, mcp.NewErrorResponse("", mcp.CodeParseError, "JSON 解析失败: "+err.Error()))
		return
	}
	resp := h.server.Handle(ctx, instanceID, &req)
	if resp == nil {
		// 通知类请求,无响应
		c.Status(consts.StatusNoContent)
		return
	}
	c.JSON(consts.StatusOK, resp)
}

// sse 处理 SSE 长连接(GET)。
// 从 hub 订阅该实例的变更通知,实时推送 tools/list_changed / agents/list_changed。
// 使用 Hertz 官方 sse.Writer(chunked + hijack),事件实时 Flush 给客户端。
// 连接关闭(客户端断开/上下文取消)时取消订阅。
func (h *MCPHandler) sse(ctx context.Context, c *app.RequestContext) {
	instanceID := c.Param("instance_id")

	ch, cancel := h.hub.Subscribe(instanceID)
	defer cancel()

	writer := hzss.NewWriter(c)
	// 初始连接确认事件
	if err := writer.WriteEvent("", "connected", []byte("connected")); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case n, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(n)
			if err != nil {
				continue
			}
			if err := writer.WriteEvent("", "message", data); err != nil {
				return
			}
		}
	}
}
