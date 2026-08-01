// Package types 定义 Agent 及其状态、会话上下文的类型体系。
// Agent 是参与对话的最小单元(游戏角色 / 系统),持有自身状态与
// 每个 session 独立的对话上下文;AgentRegistry 提供多 Agent 的统一管理。
package types

import "fmt"

// ── Agent 类型 ──────────────────────────────────────────────────────────────

// AgentType 区分 Agent 的种类。
type AgentType string

const (
	// AgentTypeActor 游戏角色(NPC / 玩家角色等),参与剧情与互动
	AgentTypeActor AgentType = "actor"
	// AgentTypeSystem 系统(GM / 仲裁 / 环境模拟等),负责规则与全局逻辑
	AgentTypeSystem AgentType = "system"
)

// ── 会话上下文 ──────────────────────────────────────────────────────────────

// SessionContext 单个会话的上下文,对应一名玩家(或外部调用方)独立的对话历史。
// Messages 复用 Message 接口,兼容 System / User / Assistant / Tool 四种消息。
type SessionContext struct {
	// SessionID 会话唯一标识(如玩家 ID、房间 ID)
	SessionID string `json:"session_id"`
	// Messages 该会话独立的对话历史
	Messages []Message `json:"messages"`
}

// NewSessionContext 构造带 ID 的空会话上下文。
func NewSessionContext(sessionID string) *SessionContext {
	return &SessionContext{SessionID: sessionID}
}

// ── Agent 状态 ──────────────────────────────────────────────────────────────

// AgentState 描述一个 Agent 的完整运行时状态。
// 同一 Agent 可同时服务多个 session,各会话上下文互相隔离。
type AgentState struct {
	// Sessions 按 SessionID 索引的会话上下文
	Sessions map[string]*SessionContext
}

// NewAgentState 构造空的 Agent 状态。
func NewAgentState() *AgentState {
	return &AgentState{Sessions: make(map[string]*SessionContext)}
}

// EnsureSession 幂等获取指定会话;不存在时自动创建并返回。
func (s *AgentState) EnsureSession(sessionID string) *SessionContext {
	if s.Sessions == nil {
		s.Sessions = make(map[string]*SessionContext)
	}
	ctx := s.Sessions[sessionID]
	if ctx == nil {
		ctx = NewSessionContext(sessionID)
		s.Sessions[sessionID] = ctx
	}
	return ctx
}

// GetSession 返回指定会话;不存在时返回 nil。
func (s *AgentState) GetSession(sessionID string) *SessionContext {
	if s.Sessions == nil {
		return nil
	}
	return s.Sessions[sessionID]
}

// RemoveSession 移除指定会话,返回是否存在。
func (s *AgentState) RemoveSession(sessionID string) bool {
	if s.Sessions == nil {
		return false
	}
	if _, ok := s.Sessions[sessionID]; !ok {
		return false
	}
	delete(s.Sessions, sessionID)
	return true
}

// ── Agent ───────────────────────────────────────────────────────────────────

// Agent 是参与对话的最小单元,存储自身状态与每个 session 的上下文。
type Agent struct {
	// Name Agent 名称,同一 AgentRegistry 内唯一
	Name string `json:"name"`
	// Type Agent 种类:actor(游戏角色)或 system(系统)
	Type AgentType `json:"type"`
	// SystemPrompt 人格与行为准则描述,作为对话历史的 system 消息基底
	SystemPrompt string `json:"system_prompt,omitempty"`
	// Tools 该 Agent 自身可调用的 tool 列表。
	// 不同 Agent 的能力不同,由此字段决定其可调用范围。
	Tools []Tool `json:"tools,omitempty"`
	// State Agent 的运行时状态(各 session 上下文)
	State *AgentState `json:"state"`
}

// AgentOption 是 NewAgent 的函数式选项,用于扩展 Agent 的初始配置。
// 后续新增能力(如 Tools、LLM 配置)时,只需增加对应的 With 选项函数。
type AgentOption func(*Agent)

// WithSystemPrompt 设置 Agent 的 system prompt(人格与行为准则)。
func WithSystemPrompt(prompt string) AgentOption {
	return func(a *Agent) {
		a.SystemPrompt = prompt
	}
}

// WithSession 预置一个初始会话上下文(携带已有对话历史时适用)。
func WithSession(ctx *SessionContext) AgentOption {
	return func(a *Agent) {
		if ctx == nil {
			return
		}
		a.State.EnsureSession(ctx.SessionID).Messages = ctx.Messages
	}
}

// WithTools 设置该 Agent 自身可调用的 tool 列表。
func WithTools(tools ...Tool) AgentOption {
	return func(a *Agent) {
		a.Tools = tools
	}
}

// SetTools 热更新该 Agent 的可调用 tool 列表。
// 与 WithTools 不同,SetTools 是运行时可变接口,
// 供 UE5 侧动态更新工具定义后同步到 Agent 的可调用范围。
func (a *Agent) SetTools(tools ...Tool) {
	a.Tools = tools
}

// SetSystemPrompt 热更新该 Agent 的角色 prompt。
// 供 UE5 侧动态更新 agent 定义后同步。
func (a *Agent) SetSystemPrompt(prompt string) {
	a.SystemPrompt = prompt
}

// NewAgent 构造一个 Agent。
// opts 可省略,此时使用默认空状态。
func NewAgent(name string, agentType AgentType, opts ...AgentOption) *Agent {
	a := &Agent{
		Name:  name,
		Type:  agentType,
		State: NewAgentState(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ── Agent 会话操作(功能完整性)─────────────────────────────────────────────

// EnsureSession 幂等获取该 Agent 的指定会话;不存在时自动创建。
func (a *Agent) EnsureSession(sessionID string) *SessionContext {
	if a.State == nil {
		a.State = NewAgentState()
	}
	return a.State.EnsureSession(sessionID)
}

// AppendMessage 向指定会话追加一条消息;会话不存在时自动创建。
func (a *Agent) AppendMessage(sessionID string, msg Message) {
	ctx := a.EnsureSession(sessionID)
	ctx.Messages = append(ctx.Messages, msg)
}

// GetMessages 返回指定会话的对话历史;会话不存在时返回 nil。
func (a *Agent) GetMessages(sessionID string) []Message {
	ctx := a.GetSession(sessionID)
	if ctx == nil {
		return nil
	}
	return ctx.Messages
}

// GetSession 返回指定会话;不存在时返回 nil。
func (a *Agent) GetSession(sessionID string) *SessionContext {
	if a.State == nil {
		return nil
	}
	return a.State.GetSession(sessionID)
}

// RemoveSession 移除指定会话,返回是否存在。
func (a *Agent) RemoveSession(sessionID string) bool {
	if a.State == nil {
		return false
	}
	return a.State.RemoveSession(sessionID)
}

// ── 多 Agent 管理 ───────────────────────────────────────────────────────────

// AgentRegistry 统一管理一组 Agent(如一场游戏的所有 NPC 与系统 Agent)。
// 保证 Agent 名称在注册表内唯一。
type AgentRegistry struct {
	agents map[string]*Agent
}

// NewAgentRegistry 构造空的 Agent 注册表。
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{agents: make(map[string]*Agent)}
}

// Register 注册一个 Agent;名称冲突时返回错误。
func (r *AgentRegistry) Register(a *Agent) error {
	if a == nil {
		return fmt.Errorf("不能注册 nil Agent")
	}
	if a.Name == "" {
		return fmt.Errorf("Agent 名称不能为空")
	}
	if _, ok := r.agents[a.Name]; ok {
		return fmt.Errorf("Agent %q 已注册", a.Name)
	}
	r.agents[a.Name] = a
	return nil
}

// Get 按名称获取 Agent;不存在时返回 (nil, false)。
func (r *AgentRegistry) Get(name string) (*Agent, bool) {
	a, ok := r.agents[name]
	return a, ok
}

// All 返回注册表内全部 Agent,顺序不保证。
func (r *AgentRegistry) All() []*Agent {
	all := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		all = append(all, a)
	}
	return all
}
