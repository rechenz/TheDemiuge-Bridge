package registry

import "github.com/rechenz/TheDemiuge-Bridge/internal/types"

// 本文件定义 YAML 配置文件的解码结构(DTO)。
// Agent / Tool 注册均通过 YAML 文件读写,由 load.go 加载并注册到类型注册表。

// ── tools.yaml ──────────────────────────────────────────────────────────────

// toolFile 对应 config/tools.yaml 的根结构。
type toolFile struct {
	// Tools 全局注册的 tool 定义列表
	Tools []toolDef `yaml:"tools"`
}

// toolDef YAML 中的单个 tool 定义。
type toolDef struct {
	// Name 工具名称(全局唯一),作为 Agent 引用绑定的标识
	Name string `yaml:"name"`
	// Description 工具功能描述,供模型理解何时以及如何调用
	Description string `yaml:"description"`
	// Parameters 工具输入参数,以 JSON Schema 对象描述;省略表示无参数
	Parameters *parametersDef `yaml:"parameters"`
	// Strict 为 true 时启用 strict 模式,默认 false
	Strict bool `yaml:"strict"`
}

// parametersDef 对应请求侧 FunctionParameters 的 YAML 描述。
type parametersDef struct {
	// Type 固定为 object
	Type string `yaml:"type"`
	// Properties 各属性名的 JSON Schema 定义
	Properties map[string]*schemaDef `yaml:"properties"`
	// Required 必填属性名列表
	Required []string `yaml:"required"`
	// AdditionalProperties 是否允许 schema 之外的额外属性
	AdditionalProperties *bool `yaml:"additional_properties"`
}

// schemaDef 递归描述属性类型的 JSON Schema 定义(YAML 版)。
type schemaDef struct {
	// Type 属性类型:string / number / integer / boolean / array / object
	Type string `yaml:"type"`
	// Description 属性描述
	Description string `yaml:"description"`
	// Enum 枚举取值约束
	Enum []any `yaml:"enum"`
	// Items array 类型时描述元素类型
	Items *schemaDef `yaml:"items"`
	// Properties object 类型时描述各属性
	Properties map[string]*schemaDef `yaml:"properties"`
	// Required object 类型时的必填属性名列表
	Required []string `yaml:"required"`
	// AdditionalProperties 是否允许额外属性
	AdditionalProperties *bool `yaml:"additional_properties"`
}

// ── agents.yaml ─────────────────────────────────────────────────────────────

// agentFile 对应 config/agents.yaml 的根结构。
type agentFile struct {
	// Agents 注册的 agent 定义列表
	Agents []agentDef `yaml:"agents"`
}

// agentDef YAML 中的单个 agent 定义。
type agentDef struct {
	// Name Agent 名称,同一注册表内唯一
	Name string `yaml:"name"`
	// Type Agent 种类:actor(游戏角色)或 system(系统)
	Type string `yaml:"type"`
	// SystemPrompt 人格与行为准则描述
	SystemPrompt string `yaml:"system_prompt"`
	// Tools 该 Agent 可调用的 tool 名称列表(引用 tools.yaml 中已注册的工具)
	Tools []string `yaml:"tools"`
}

// ── 转换 ────────────────────────────────────────────────────────────────────

// toFunctionParameters 将 YAML DTO 转换为请求侧类型。
func (d *parametersDef) toFunctionParameters() *types.FunctionParameters {
	if d == nil {
		return nil
	}
	return &types.FunctionParameters{
		Type:                 d.Type,
		Properties:           toJSONSchemaMap(d.Properties),
		Required:             d.Required,
		AdditionalProperties: d.AdditionalProperties,
	}
}

// toJSONSchemaMap 递归转换属性集合。
func toJSONSchemaMap(m map[string]*schemaDef) map[string]*types.JSONSchema {
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
func (d *schemaDef) toJSONSchema() *types.JSONSchema {
	if d == nil {
		return nil
	}
	return &types.JSONSchema{
		Type:                 d.Type,
		Description:          d.Description,
		Enum:                 d.Enum,
		Items:                d.Items.toJSONSchema(),
		Properties:           toJSONSchemaMap(d.Properties),
		Required:             d.Required,
		AdditionalProperties: d.AdditionalProperties,
	}
}
