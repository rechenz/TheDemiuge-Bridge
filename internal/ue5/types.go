// Package ue5 实现 UE5 实例接入层。
//
// 每个 UE5 游戏服务器实例是一个独立的注册单元:通过外部管理接口
// 动态注册/更新/删除 agent 与 tool 定义,Bridge 侧按实例隔离存储并
// 转发工具调用。所有注册信息可落盘持久化,Bridge 重启后自动恢复。
package ue5

import (
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ── Tool 注册 ───────────────────────────────────────────────────────────────

// ToolReg 一个 UE5 实例注册的工具注册条目。
// 包含工具定义(供 MCP tools/list 与 LLM tool calling 使用)
// 以及转发的执行地址(UE5 侧实际执行端)。
// 同时用于 UE5 上传、实例落盘与内存存储,字段平铺。
type ToolReg struct {
	ToolDef
	// Endpoint 工具执行时的转发地址。
	// 为空时使用实例级别的默认转发地址。
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	// TimeoutMS 单次转发的超时时间(毫秒)。
	// 0 表示使用全局默认转发超时。
	TimeoutMS int `json:"timeout_ms,omitempty" yaml:"timeout_ms,omitempty"`
}

// ToTool 返回请求侧 types.Tool(供 MCP tools/list 与 LLM tool calling 使用)。
func (r ToolReg) ToTool() types.Tool {
	return r.ToolDef.ToTool()
}

// ── Agent 注册 ─────────────────────────────────────────────────────────────

// AgentDef 一个 UE5 实例注册的 agent 定义。
// 对应单个 agent 的注册文件(如 registry/{instance}/agents/{name}.yaml)。
type AgentDef struct {
	// Name agent 名称,实例内唯一
	Name string `json:"name" yaml:"name"`
	// Type agent 种类:actor(游戏角色)或 system(系统)
	Type string `json:"type" yaml:"type"`
	// SystemPrompt 人格与行为准则描述
	SystemPrompt string `json:"system_prompt,omitempty" yaml:"system_prompt,omitempty"`
	// Tools 该 agent 可调用的 tool 名称列表(引用实例 tools.yaml 中已注册的工具)
	Tools []string `json:"tools,omitempty" yaml:"tools,omitempty"`
}

// ── 实例信息 ───────────────────────────────────────────────────────────────

// InstanceInfo 一个 UE5 实例的概要信息,供查询接口返回。
type InstanceInfo struct {
	// ID 实例唯一标识
	ID string `json:"id"`
	// DefaultEndpoint 实例级别的默认转发地址
	DefaultEndpoint string `json:"default_endpoint,omitempty"`
	// AgentCount 当前已注册的 agent 数量
	AgentCount int `json:"agent_count"`
	// ToolCount 当前已注册的 tool 数量
	ToolCount int `json:"tool_count"`
}

// ── 类型转换辅助 ───────────────────────────────────────────────────────────

// ToolDef 对应单个工具注册文件/请求体的工具定义部分
// (与 config/tools.yaml 的字段一致,供 UE5 上传时解码)。
type ToolDef struct {
	// Name 工具名称(实例内唯一)
	Name string `json:"name" yaml:"name"`
	// Description 工具功能描述
	Description string `json:"description" yaml:"description"`
	// Parameters 工具输入参数 JSON Schema(省略表示无参数)
	Parameters *toolParams `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	// Strict 是否启用 strict 模式
	Strict bool `json:"strict,omitempty" yaml:"strict,omitempty"`
}

// toolParams 对应 ToolDef 的参数描述(与 registry.decode 的 parametersDef 同构,
// 因包私有不可复用,此处独立定义)。
type toolParams struct {
	Type                 string                 `json:"type" yaml:"type"`
	Properties           map[string]*toolSchema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Required             []string               `json:"required,omitempty" yaml:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additional_properties,omitempty" yaml:"additional_properties,omitempty"`
}

// toolSchema 递归描述属性类型的 JSON Schema。
type toolSchema struct {
	Type                 string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Description          string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Enum                 []any                  `json:"enum,omitempty" yaml:"enum,omitempty"`
	Items                *toolSchema            `json:"items,omitempty" yaml:"items,omitempty"`
	Properties           map[string]*toolSchema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Required             []string               `json:"required,omitempty" yaml:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additional_properties,omitempty" yaml:"additional_properties,omitempty"`
}

// ToTool 将 ToolDef 转换为请求侧 types.Tool。
func (d ToolDef) ToTool() types.Tool {
	return types.Tool{
		Type: types.ToolTypeFunction,
		Function: types.Function{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters.toFunctionParameters(),
			Strict:      d.Strict,
		},
	}
}

// toFunctionParameters 将 YAML/JSON DTO 转换为请求侧类型。
func (p *toolParams) toFunctionParameters() *types.FunctionParameters {
	if p == nil {
		return nil
	}
	return &types.FunctionParameters{
		Type:                 p.Type,
		Properties:           toJSONSchemaMap(p.Properties),
		Required:             p.Required,
		AdditionalProperties: p.AdditionalProperties,
	}
}

// toJSONSchemaMap 递归转换属性集合。
func toJSONSchemaMap(m map[string]*toolSchema) map[string]*types.JSONSchema {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*types.JSONSchema, len(m))
	for k, v := range m {
		out[k] = v.toJSONSchema()
	}
	return out
}

// toJSONSchema 递归转换单个属性定义。
func (s *toolSchema) toJSONSchema() *types.JSONSchema {
	if s == nil {
		return nil
	}
	return &types.JSONSchema{
		Type:                 s.Type,
		Description:          s.Description,
		Enum:                 s.Enum,
		Items:                s.Items.toJSONSchema(),
		Properties:           toJSONSchemaMap(s.Properties),
		Required:             s.Required,
		AdditionalProperties: s.AdditionalProperties,
	}
}

// FromTool 将请求侧 types.Tool 转换为 ToolDef(供查询接口返回)。
func FromTool(t types.Tool) ToolDef {
	return ToolDef{
		Name:        t.Function.Name,
		Description: t.Function.Description,
		Parameters:  fromFunctionParameters(t.Function.Parameters),
		Strict:      t.Function.Strict,
	}
}

// fromFunctionParameters 将请求侧类型转换为 DTO。
func fromFunctionParameters(p *types.FunctionParameters) *toolParams {
	if p == nil {
		return nil
	}
	return &toolParams{
		Type:                 p.Type,
		Properties:           fromJSONSchemaMap(p.Properties),
		Required:             p.Required,
		AdditionalProperties: p.AdditionalProperties,
	}
}

func fromJSONSchemaMap(m map[string]*types.JSONSchema) map[string]*toolSchema {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*toolSchema, len(m))
	for k, v := range m {
		out[k] = fromJSONSchema(v)
	}
	return out
}

func fromJSONSchema(s *types.JSONSchema) *toolSchema {
	if s == nil {
		return nil
	}
	return &toolSchema{
		Type:                 s.Type,
		Description:          s.Description,
		Enum:                 s.Enum,
		Items:                fromJSONSchema(s.Items),
		Properties:           fromJSONSchemaMap(s.Properties),
		Required:             s.Required,
		AdditionalProperties: s.AdditionalProperties,
	}
}
