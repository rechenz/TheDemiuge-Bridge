package mcp

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ── 通用注册描述类型 ───────────────────────────────────────────────────────

// RegisteredAgent 描述一个注册的 agent(协议层通用表示)。
// 由具体后端适配器从自己的注册数据转换而来,MCP 层仅消费此描述。
// 工具定义直接复用 types.Tool(与 LLM tool calling 同一表示),不再独立建模。
type RegisteredAgent struct {
	// Name agent 名称(实例内唯一)
	Name string
	// Type agent 种类:actor(游戏角色)或 system(系统)
	Type string
	// SystemPrompt 人格与行为准则描述
	SystemPrompt string
	// ToolNames 该 agent 可调用的工具名称列表
	ToolNames []string
}

// ── 注册变更 ────────────────────────────────────────────────────────────────

// ChangeKind 一次注册变更的种类。
type ChangeKind string

const (
	// ChangeTool 工具变更(tools/list_changed 广播)
	ChangeTool ChangeKind = "tool"
	// ChangeAgent agent 变更(agents/list_changed 广播)
	ChangeAgent ChangeKind = "agent"
)

// Change 一次注册变更信息。
// Kind 为变更种类,InstanceID 与 Name 定位被变更的条目。
type Change struct {
	Kind       ChangeKind
	InstanceID string
	Name       string
}

// ── Registry 接口 ───────────────────────────────────────────────────────────

// Registry 是 MCP 协议层依赖的注册中心接口。
// 由具体后端(如 UE5)实现:
//   - 数据面:Tools/Agents 系列方法从后端注册空间实时读取;
//   - 执行面:ExecuteTool 按后端协议执行工具
//     (UE5 走 HTTP 转发,本地函数走直接调用,其他引擎走各自连接协议)。
//
// 该接口使 MCP 层与任何特定后端解耦——"连接什么由注册实现"。
type Registry interface {
	// Tools 返回实例的全部工具;实例不存在时返回 (nil, false)。
	// 工具以 types.Tool 表示(与 LLM tool calling 同一格式)。
	Tools(instanceID string) ([]types.Tool, bool)
	// GetTool 按名称取工具;不存在时返回 (types.Tool{}, false)。
	GetTool(instanceID, name string) (types.Tool, bool)
	// Agents 返回实例的全部 agent;实例不存在时返回 (nil, false)。
	Agents(instanceID string) ([]RegisteredAgent, bool)
	// GetAgent 按名称取 agent;不存在时返回 (RegisteredAgent{}, false)。
	GetAgent(instanceID, name string) (RegisteredAgent, bool)
	// ExecuteTool 执行一次工具调用并返回结果文本。
	// 参数 args 为模型生成的 JSON 对象;执行失败返回错误,
	// 由调用方决定是直接失败还是把错误作为结果回馈模型。
	ExecuteTool(ctx context.Context, instanceID, name string, args map[string]any) (string, error)
}
