package types

// Role 表示一条对话消息的发起角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 是所有对话消息的统一接口。
// 各具体类型通过 GetRole / GetContent 暴露只读访问,
// isMessage 保证只有本包内定义的消息类型可实现该接口。
type Message interface {
	GetRole() string
	GetContent() string

	isMessage()
}

// ── System 消息 ─────────────────────────────────────────────────────────────

// SystemMessage 对应文档的 System message,通常用于设定助手行为与人格。
type SystemMessage struct {
	// Content system 消息的内容(必填)
	Content string `json:"content"`
	// Name 可选填的参与者名称,用于区分相同角色的参与者
	Name string `json:"name,omitempty"`
}

func (m SystemMessage) GetRole() string    { return string(RoleSystem) }
func (m SystemMessage) GetContent() string { return m.Content }
func (m SystemMessage) isMessage()         {}

// ── User 消息 ───────────────────────────────────────────────────────────────

// UserMessage 对应文档的 User message。
// 当前 Content 以 string 承载文本;后续如需支持多模态内容
// (图片 / 音频等 ContentPart 数组),可在此扩展 ContentParts 字段。
type UserMessage struct {
	// Content user 消息的内容(必填)
	Content string `json:"content"`
	// Name 可选填的参与者名称,用于区分相同角色的参与者
	Name string `json:"name,omitempty"`
}

func (m UserMessage) GetRole() string    { return string(RoleUser) }
func (m UserMessage) GetContent() string { return m.Content }
func (m UserMessage) isMessage()         {}

// ── Assistant 消息 ──────────────────────────────────────────────────────────

// AssistantMessage 同时承担两种职责:
//  1. 请求侧:作为多轮对话历史中的 assistant 消息回传;
//  2. 响应侧:作为非流式响应 choices[i].message 的解码目标。
//
// 对应文档的 Assistant message。
type AssistantMessage struct {
	// Content assistant 消息的内容,nullable。
	// 当模型仅发起 tool 调用(content 为 null)时必须为 nil。
	Content *string `json:"content"`
	// ReasoningContent 仅适用于思考模式,
	// 为 assistant 消息中在最终答案之前的推理内容,nullable。
	ReasoningContent *string `json:"reasoning_content,omitempty"`
	// ToolCalls 模型生成的 tool 调用(如 function 调用)。
	// 多轮对话时,需将模型响应中的 tool_calls 原样回传。
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Name 可选填的参与者名称,用于区分相同角色的参与者
	Name string `json:"name,omitempty"`
	// Prefix (Beta) 设置为 true 时强制模型以其内容作为回答前缀开始;
	// 需使用 base_url="https://api.deepseek.com/beta"
	Prefix *bool `json:"prefix,omitempty"`
}

// NewAssistantMessage 构造普通文本 assistant 消息。
func NewAssistantMessage(content string) AssistantMessage {
	c := content
	return AssistantMessage{Content: &c}
}

// NewAssistantMessageWithReasoning 构造带推理内容的 assistant 消息。
func NewAssistantMessageWithReasoning(content, reasoning string) AssistantMessage {
	c := content
	r := reasoning
	return AssistantMessage{Content: &c, ReasoningContent: &r}
}

// NewAssistantMessageWithToolCalls 构造仅包含 tool 调用的 assistant 消息。
func NewAssistantMessageWithToolCalls(calls ...ToolCall) AssistantMessage {
	return AssistantMessage{ToolCalls: calls}
}

func (m AssistantMessage) GetRole() string { return string(RoleAssistant) }
func (m AssistantMessage) GetContent() string {
	if m.Content == nil {
		return ""
	}
	return *m.Content
}
func (m AssistantMessage) isMessage() {}

// ── Tool 消息 ───────────────────────────────────────────────────────────────

// ToolMessage 对应文档的 Tool message,用于回传工具执行结果。
type ToolMessage struct {
	// Content tool 消息的内容(必填)
	Content string `json:"content"`
	// ToolCallID 此消息所响应的 tool call 的 ID(必填)
	ToolCallID string `json:"tool_call_id"`
}

func (m ToolMessage) GetRole() string    { return string(RoleTool) }
func (m ToolMessage) GetContent() string { return m.Content }
func (m ToolMessage) isMessage()         {}
