// Package mcp 实现 MCP(Model Context Protocol)协议核心。
//
// Bridge 以 MCP Server 身份对外暴露 UE5 实例封装的能力:
//   - tools/list     返回某 UE5 实例注册的全部工具(tools.yaml)
//   - tools/call     转发到 UE5 侧执行并返回结果
//   - prompts/list   返回某 UE5 实例注册的全部 agent(映射为 prompt 模板)
//   - prompts/get    按 agent 名返回其 system_prompt(供外部 agent 扮演该 NPC)
//   - ping           存活探测
//
// 传输层使用 JSON-RPC 2.0 over HTTP POST(可增量升级 SSE 长连接)。
package mcp

import "encoding/json"

// ── JSON-RPC 2.0 ────────────────────────────────────────────────────────────

// RPCVersion JSON-RPC 2.0 版本号。
const RPCVersion = "2.0"

// Request 一条 JSON-RPC 请求。
// ID 字符串,支持批量消息(MCP 允许可空,本实现要求必填以方便关联响应)。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Notification 一条 JSON-RPC 通知(无 ID)。
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response 一条 JSON-RPC 响应。
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	// Result 成功结果;失败时为 nil
	Result json.RawMessage `json:"result,omitempty"`
	// Error 失败信息;成功时为 nil
	Error *RPCError `json:"error,omitempty"`
}

// RPCError JSON-RPC 错误对象。
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC 错误码。
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// ── MCP 方法 ────────────────────────────────────────────────────────────────

// MCP 方法名常量。
const (
	MethodInitialize         = "initialize"
	MethodNotifInitialized   = "notifications/initialized"
	MethodPing               = "ping"
	MethodToolsList          = "tools/list"
	MethodToolsCall          = "tools/call"
	MethodPromptsList        = "prompts/list"
	MethodPromptsGet         = "prompts/get"
	MethodNotifToolsChanged  = "notifications/tools/list_changed"
	MethodNotifAgentsChanged = "notifications/agents/list_changed"
)

// ── 协议翻牌结果 ────────────────────────────────────────────────────────────

// ServerInfo MCP server 自身信息。
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo MCP client 信息(从 initialize 请求读取)。
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitParams initialize 请求的参数。
type InitParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    any        `json:"capabilities,omitempty"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// InitResult initialize 返回的 server 能力声明。
type InitResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ServerCaps `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
	Instructions    string     `json:"instructions,omitempty"`
}

// ServerCaps MCP server 能力集。
type ServerCaps struct {
	Tools   *ToolsCaps   `json:"tools,omitempty"`
	Prompts *PromptsCaps `json:"prompts,omitempty"`
}

// ToolsCaps 工具能力。
type ToolsCaps struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptsCaps prompt 能力。
type PromptsCaps struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ── tools/list ──────────────────────────────────────────────────────────────

// Tool MCP 协议层的工具表示。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolsListResult tools/list 的返回。
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ── tools/call ──────────────────────────────────────────────────────────────

// ToolsCallParams tools/call 的参数。
type ToolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// CallResultContent 工具调用的结构化内容(每个元素可含 text/image/resource)。
type CallResultContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolsCallResult tools/call 的返回。
// IsError 为 true 表示工具执行失败(结果作为错误内容返回给客户端)。
type ToolsCallResult struct {
	Content []CallResultContent `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

// ── prompts/list 与 prompts/get ─────────────────────────────────────────────

// PromptArg prompt 模板的参数声明。
type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt MCP 协议层的 prompt 模板表示。
type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
}

// PromptsListResult prompts/list 的返回。
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// PromptsGetParams prompts/get 的参数。
type PromptsGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptMessage prompt 消息段。
type PromptMessage struct {
	Role    string `json:"role"`
	Content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// PromptsGetResult prompts/get 的返回。
type PromptsGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// ── 便捷构造 ────────────────────────────────────────────────────────────────

// NewResponse 构造成功响应。
func NewResponse(id string, result any) *Response {
	data, err := json.Marshal(result)
	if err != nil {
		return NewErrorResponse(id, CodeInternalError, "序列化 result 失败: "+err.Error())
	}
	return &Response{JSONRPC: RPCVersion, ID: id, Result: data}
}

// NewErrorResponse 构造错误响应。
func NewErrorResponse(id string, code int, message string) *Response {
	return &Response{
		JSONRPC: RPCVersion,
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
}
