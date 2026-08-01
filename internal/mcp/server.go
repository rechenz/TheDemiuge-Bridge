package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// ServerInfo 默认 server 标识。
const (
	ServerName    = "TheDemiuge-Bridge"
	ServerVersion = "0.1.0"
	// ProtocolVersion 支持的 MCP 协议版本。
	ProtocolVersion = "2025-06-18"
)

// Server 是 MCP 协议分发器。
// 每个请求携带 instanceID,从对应 UE5 实例的注册空间读取工具与 agent。
// 工具调用通过 ue5.Client 转发到 UE5 侧实际执行。
type Server struct {
	mgr *ue5.Manager
	cli *ue5.Client
}

// NewServer 构造 MCP Server。
func NewServer(mgr *ue5.Manager, cli *ue5.Client) *Server {
	return &Server{mgr: mgr, cli: cli}
}

// Handle 处理一条 JSON-RPC 请求并返回响应。
// 无 ID 的通知返回 nil(调用方不回复)。
// 分发失败(未知方法/参数非法/执行错误)时返回带 error 的响应。
func (s *Server) Handle(ctx context.Context, instanceID string, req *Request) *Response {
	if req == nil {
		return NewErrorResponse("", CodeInvalidRequest, "请求为空")
	}
	if req.Method == "" {
		return NewErrorResponse(req.ID, CodeInvalidRequest, "缺少 method")
	}

	switch req.Method {
	case MethodInitialize:
		return s.handleInitialize(req)
	case MethodPing:
		return NewResponse(req.ID, map[string]any{})
	case MethodToolsList:
		return s.handleToolsList(instanceID, req)
	case MethodToolsCall:
		return s.handleToolsCall(ctx, instanceID, req)
	case MethodPromptsList:
		return s.handlePromptsList(instanceID, req)
	case MethodPromptsGet:
		return s.handlePromptsGet(instanceID, req)
	default:
		return NewErrorResponse(req.ID, CodeMethodNotFound, "未知方法: "+req.Method)
	}
}

// ── initialize ──────────────────────────────────────────────────────────────

func (s *Server) handleInitialize(req *Request) *Response {
	var params InitParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewErrorResponse(req.ID, CodeInvalidParams, "initialize 参数非法: "+err.Error())
		}
	}
	// 广播能力(供 SSE 长连接使用)
	result := InitResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCaps{
			Tools:   &ToolsCaps{ListChanged: true},
			Prompts: &PromptsCaps{ListChanged: true},
		},
		ServerInfo: ServerInfo{Name: ServerName, Version: ServerVersion},
		Instructions: "通过 tools/* 调用 UE5 注册的动画/通信能力;" +
			"通过 prompts/* 获取 NPC 角色 prompt 以扮演该角色。",
	}
	return NewResponse(req.ID, result)
}

// ── tools/list ──────────────────────────────────────────────────────────────

func (s *Server) handleToolsList(instanceID string, req *Request) *Response {
	tools := s.mgr.Tools(instanceID)
	if tools == nil {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("实例 %q 不存在", instanceID))
	}

	out := make([]Tool, 0, len(tools))
	for _, reg := range tools {
		out = append(out, toProtocolTool(reg.ToTool()))
	}
	return NewResponse(req.ID, ToolsListResult{Tools: out})
}

// ── tools/call ──────────────────────────────────────────────────────────────

func (s *Server) handleToolsCall(ctx context.Context, instanceID string, req *Request) *Response {
	var params ToolsCallParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewErrorResponse(req.ID, CodeInvalidParams, "tools/call 参数非法: "+err.Error())
		}
	}
	if params.Name == "" {
		return NewErrorResponse(req.ID, CodeInvalidParams, "缺少工具名称 name")
	}

	inst, ok := s.mgr.GetInstance(instanceID)
	if !ok {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("实例 %q 不存在", instanceID))
	}
	reg, ok := s.mgr.GetTool(instanceID, params.Name)
	if !ok {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("工具 %q 未在实例 %q 中注册", params.Name, instanceID))
	}

	// 校验参数不为 nil(UE5 侧要求 JSON 对象)
	args := params.Arguments
	if args == nil {
		args = map[string]any{}
	}

	resp, err := s.cli.Forward(ctx, inst, reg, args)
	if err != nil {
		return NewErrorResponse(req.ID, CodeInternalError, "工具执行失败: "+err.Error())
	}
	text, err := resp.Text()
	if err != nil {
		return NewErrorResponse(req.ID, CodeInternalError, "解析工具结果失败: "+err.Error())
	}
	result := ToolsCallResult{
		Content: []CallResultContent{{Type: "text", Text: text}},
	}
	return NewResponse(req.ID, result)
}

// ── prompts/list ────────────────────────────────────────────────────────────

func (s *Server) handlePromptsList(instanceID string, req *Request) *Response {
	agents := s.mgr.Agents(instanceID)
	if agents == nil {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("实例 %q 不存在", instanceID))
	}

	out := make([]Prompt, 0, len(agents))
	for _, def := range agents {
		args := make([]PromptArg, 0, 1)
		args = append(args, PromptArg{Name: "player_message", Description: "玩家对 NPC 说的一句话", Required: true})
		out = append(out, Prompt{
			Name:        def.Name,
			Description: fmt.Sprintf("扮演 %s(类型 %s)", def.Name, def.Type),
			Arguments:   args,
		})
	}
	return NewResponse(req.ID, PromptsListResult{Prompts: out})
}

// ── prompts/get ─────────────────────────────────────────────────────────────

func (s *Server) handlePromptsGet(instanceID string, req *Request) *Response {
	var params PromptsGetParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewErrorResponse(req.ID, CodeInvalidParams, "prompts/get 参数非法: "+err.Error())
		}
	}
	if params.Name == "" {
		return NewErrorResponse(req.ID, CodeInvalidParams, "缺少 prompt 名称 name")
	}

	def, ok := s.mgr.GetAgent(instanceID, params.Name)
	if !ok {
		return NewErrorResponse(req.ID, CodeInvalidParams, fmt.Sprintf("agent %q 未在实例 %q 中注册", params.Name, instanceID))
	}

	// 组装 system prompt + 可用的工具清单(供外部 agent 了解可调用能力)
	text := def.SystemPrompt
	if text == "" {
		text = fmt.Sprintf("你是 %s。", def.Name)
	}
	for _, toolName := range def.Tools {
		text += fmt.Sprintf("\n- 你可用工具: %s", toolName)
	}

	userText := "{{player_message}}"
	if playerMsg, ok := params.Arguments["player_message"]; ok && playerMsg != "" {
		userText = playerMsg
	}

	msg := PromptsGetResult{Description: fmt.Sprintf("NPC %s 的角色 prompt", def.Name)}
	msg.Messages = append(msg.Messages,
		PromptMessage{Role: "system", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: text}},
		PromptMessage{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: userText}},
	)
	return NewResponse(req.ID, msg)
}

// ── 类型转换 ────────────────────────────────────────────────────────────────

// toProtocolTool 将请求侧 types.Tool 转换为 MCP 协议层 Tool。
// inputSchema 为 JSON Schema object(map 形式),MCP 客户端据此生成参数。
func toProtocolTool(t types.Tool) Tool {
	schema := toInputSchema(t.Function.Parameters)
	return Tool{
		Name:        t.Function.Name,
		Description: t.Function.Description,
		InputSchema: schema,
	}
}

// toInputSchema 将 types.FunctionParameters 转换为 map[string]any 的 JSON Schema。
// nil 参数时返回空对象 {"type":"object"}。
func toInputSchema(p *types.FunctionParameters) map[string]any {
	if p == nil {
		return map[string]any{"type": "object"}
	}
	out := map[string]any{"type": "object"}
	if len(p.Properties) > 0 {
		props := make(map[string]any, len(p.Properties))
		for k, v := range p.Properties {
			props[k] = toSchemaMap(v)
		}
		out["properties"] = props
	}
	if len(p.Required) > 0 {
		out["required"] = p.Required
	}
	if p.AdditionalProperties != nil {
		out["additionalProperties"] = *p.AdditionalProperties
	}
	return out
}

// toSchemaMap 将 types.JSONSchema 递归转换为 map 形式。
func toSchemaMap(s *types.JSONSchema) map[string]any {
	if s == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if s.Type != "" {
		out["type"] = s.Type
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}
	if s.Items != nil {
		out["items"] = toSchemaMap(s.Items)
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = toSchemaMap(v)
		}
		out["properties"] = props
	}
	if len(s.Required) > 0 {
		out["required"] = s.Required
	}
	if s.AdditionalProperties != nil {
		out["additionalProperties"] = *s.AdditionalProperties
	}
	return out
}
