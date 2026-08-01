package types

import (
	"encoding/json"
	"fmt"
)

// ── Tool(请求侧定义)───────────────────────────────────────────────────────

// Tool 模型可能会调用的 tool。目前仅支持 function 作为工具,
// 最多支持 128 个 function。
type Tool struct {
	// Type tool 的类型,目前仅支持 "function"
	Type string `json:"type"`
	// Function function 的详细定义
	Function Function `json:"function"`
}

// Function function 的详细定义。
type Function struct {
	// Name 要调用的 function 名称(必填)。
	// 必须由 a-z、A-Z、0-9 组成,或包含下划线和连字符,最大长度 64。
	Name string `json:"name"`
	// Description function 的功能描述,供模型理解何时以及如何调用
	Description string `json:"description,omitempty"`
	// Parameters function 的输入参数,以 JSON Schema 对象描述。
	// 省略时定义一个参数列表为空的 function。
	Parameters *FunctionParameters `json:"parameters,omitempty"`
	// Strict (Beta) 为 true 时启用 strict 模式,
	// 确保输出始终符合 function 的 JSON schema 定义,默认 false
	Strict bool `json:"strict,omitempty"`
}

// FunctionParameters 对应 JSON Schema 中的 object 类型描述。
type FunctionParameters struct {
	// Type 固定为 object
	Type string `json:"type"`
	// Properties 各属性名的 JSON Schema 定义
	Properties map[string]*JSONSchema `json:"properties,omitempty"`
	// Required 必填属性名列表
	Required []string `json:"required,omitempty"`
	// AdditionalProperties 是否允许 schema 之外的额外属性,默认 true
	AdditionalProperties *bool `json:"additional_properties,omitempty"`
}

// NewFunctionTool 构造一个 function 类型的 Tool。
// schema 可传 nil,表示参数列表为空的 function。
func NewFunctionTool(name, description string, schema *FunctionParameters, strict bool) *Tool {
	return &Tool{
		Type: ToolTypeFunction,
		Function: Function{
			Name:        name,
			Description: description,
			Parameters:  schema,
			Strict:      strict,
		},
	}
}

// ── 基础 JSON Schema ───────────────────────────────────────────────────────

// JSONSchema 描述 function 参数的 JSON Schema 定义。
// 支持 string / number / integer / boolean / array / object 六种类型,
// 并通过 Items / Properties 递归描述复合结构。
type JSONSchema struct {
	// Type 属性类型
	Type string `json:"type,omitempty"`
	// Description 属性描述
	Description string `json:"description,omitempty"`
	// Enum 枚举取值约束(如 ["string", "number"])
	Enum []any `json:"enum,omitempty"`
	// Items array 类型时描述元素类型
	Items *JSONSchema `json:"items,omitempty"`
	// Properties object 类型时描述各属性
	Properties map[string]*JSONSchema `json:"properties,omitempty"`
	// Required object 类型时的必填属性名列表
	Required []string `json:"required,omitempty"`
	// AdditionalProperties 是否允许额外属性,默认 true
	AdditionalProperties *bool `json:"additional_properties,omitempty"`
}

// JSON Schema 类型常量。
const (
	SchemaTypeString  = "string"
	SchemaTypeNumber  = "number"
	SchemaTypeInteger = "integer"
	SchemaTypeBoolean = "boolean"
	SchemaTypeArray   = "array"
	SchemaTypeObject  = "object"
)

// NewSchemaString 构造 string 类型属性。
func NewSchemaString(description string) *JSONSchema {
	return &JSONSchema{Type: SchemaTypeString, Description: description}
}

// NewSchemaNumber 构造 number 类型属性。
func NewSchemaNumber(description string) *JSONSchema {
	return &JSONSchema{Type: SchemaTypeNumber, Description: description}
}

// NewSchemaInteger 构造 integer 类型属性。
func NewSchemaInteger(description string) *JSONSchema {
	return &JSONSchema{Type: SchemaTypeInteger, Description: description}
}

// NewSchemaBoolean 构造 boolean 类型属性。
func NewSchemaBoolean(description string) *JSONSchema {
	return &JSONSchema{Type: SchemaTypeBoolean, Description: description}
}

// NewSchemaArray 构造 array 类型属性。
func NewSchemaArray(description string, items *JSONSchema) *JSONSchema {
	return &JSONSchema{Type: SchemaTypeArray, Description: description, Items: items}
}

// NewSchemaObject 构造 object 类型属性。
func NewSchemaObject(description string, properties map[string]*JSONSchema, required []string) *JSONSchema {
	return &JSONSchema{
		Type:        SchemaTypeObject,
		Description: description,
		Properties:  properties,
		Required:    required,
	}
}

// ── 模型侧 Tool 调用 ───────────────────────────────────────────────────────

// ToolCall 模型在响应中生成的 tool 调用(如 function 调用)。
// 同时用于非流式响应的 message.tool_calls 与
// 流式 chunk 的 delta.tool_calls(增量拼接)。
type ToolCall struct {
	// Index 标识号,仅流式 delta.tool_calls 中携带,
	// 用于把同一个 function 调用的增量块(id/name/arguments 片段)归位拼接;
	// 非流式响应的 message.tool_calls 无此字段,为零值。
	Index int `json:"index,omitempty"`
	// ID tool 调用的 ID
	ID string `json:"id"`
	// Type tool 的类型,目前仅支持 "function"
	Type string `json:"type"`
	// Function 模型调用的 function
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 被调用的 function 信息。
type ToolCallFunction struct {
	// Name 模型调用的 function 名
	Name string `json:"name"`
	// Arguments function 的参数,由模型生成,格式为 JSON 字符串。
	// 注意:模型并不总是生成有效 JSON,可能在函数模式外臆造参数,
	// 调用前必须在代码中校验这些参数。
	Arguments string `json:"arguments"`
}

// UnmarshalArguments 将函数参数 JSON 字符串解码到目标结构体。
func (f ToolCallFunction) UnmarshalArguments(v any) error {
	return json.Unmarshal([]byte(f.Arguments), v)
}

// ── ToolChoice ─────────────────────────────────────────────────────────────

// ToolChoiceMode 控制模型调用 tool 行为的字符串模式。
type ToolChoiceMode string

const (
	// ToolChoiceNone 模型不会调用任何 tool,而是生成一条消息
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceAuto 模型可以选择生成一条消息或调用一个或多个 tool。
	// 没有 tools 时默认 none;有 tools 时默认 auto。
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceRequired 模型必须调用一个或多个 tool
	ToolChoiceRequired ToolChoiceMode = "required"
)

// ToolChoice 对应请求体顶层字段 tool_choice,支持文档的 oneOf 语义:
//  1. 字符串模式:none / auto / required;
//  2. 命名调用:{"type":"function","function":{"name":"my_function"}},
//     强制模型调用该 tool。
//
// 使用 NewToolChoiceXxx 构造即可,无需关心底层编码。
type ToolChoice struct {
	// Mode 为 none / auto / required 时非空
	Mode ToolChoiceMode
	// Function 为命名调用时非空
	Function *ToolChoiceFunction
}

// ToolChoiceFunction 命名调用指定 tool。
type ToolChoiceFunction struct {
	// Name 要调用的函数名称
	Name string `json:"name"`
}

// NewToolChoiceNone 构造 none 模式。
func NewToolChoiceNone() *ToolChoice {
	return &ToolChoice{Mode: ToolChoiceNone}
}

// NewToolChoiceAuto 构造 auto 模式。
func NewToolChoiceAuto() *ToolChoice {
	return &ToolChoice{Mode: ToolChoiceAuto}
}

// NewToolChoiceRequired 构造 required 模式。
func NewToolChoiceRequired() *ToolChoice {
	return &ToolChoice{Mode: ToolChoiceRequired}
}

// NewToolChoiceFunction 构造强制调用指定函数的模式。
func NewToolChoiceFunction(name string) *ToolChoice {
	return &ToolChoice{Function: &ToolChoiceFunction{Name: name}}
}

// MarshalJSON 依据构造方式输出字符串或对象形式。
func (c ToolChoice) MarshalJSON() ([]byte, error) {
	if c.Function != nil {
		return json.Marshal(struct {
			Type     string             `json:"type"`
			Function ToolChoiceFunction `json:"function"`
		}{
			Type:     ToolTypeFunction,
			Function: *c.Function,
		})
	}
	mode := c.Mode
	if mode == "" {
		mode = ToolChoiceAuto
	}
	return json.Marshal(string(mode))
}

// UnmarshalJSON 兼容字符串与对象两种输入形式。
func (c *ToolChoice) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*c = ToolChoice{Mode: ToolChoiceAuto}
		return nil
	}
	var mode string
	if err := json.Unmarshal(data, &mode); err == nil {
		switch ToolChoiceMode(mode) {
		case ToolChoiceNone, ToolChoiceAuto, ToolChoiceRequired:
			*c = ToolChoice{Mode: ToolChoiceMode(mode)}
			return nil
		default:
			return fmt.Errorf("tool_choice 字符串模式必须是 none/auto/required,得到 %q", mode)
		}
	}
	var named struct {
		Type     string             `json:"type"`
		Function ToolChoiceFunction `json:"function"`
	}
	if err := json.Unmarshal(data, &named); err != nil {
		return fmt.Errorf("tool_choice 必须是字符串模式或 {type, function} 对象: %w", err)
	}
	if named.Type != ToolTypeFunction {
		return fmt.Errorf("tool_choice.type 目前仅支持 function,得到 %q", named.Type)
	}
	*c = ToolChoice{Function: &named.Function}
	return nil
}

// ── Tool 注册中心 ───────────────────────────────────────────────────────────

// ToolRegistry 统一管理已注册的 Tool(如全局可用的能力清单)。
// 保证 Tool 名称在注册表内唯一;Agent 通过引用名称绑定自身可调用的 tool。
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry 构造空的 Tool 注册表。
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

// Register 注册一个 Tool;nil 或名称冲突时返回错误。
func (r *ToolRegistry) Register(t Tool) error {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if t.Function.Name == "" {
		return fmt.Errorf("Tool 名称不能为空")
	}
	if _, ok := r.tools[t.Function.Name]; ok {
		return fmt.Errorf("Tool %q 已注册", t.Function.Name)
	}
	r.tools[t.Function.Name] = t
	return nil
}

// Upsert 注册或覆盖一个 Tool(热更新语义)。
// 同名已存在时直接覆盖,用于 UE5 侧动态更新工具定义。
func (r *ToolRegistry) Upsert(t Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if t.Function.Name == "" {
		return
	}
	r.tools[t.Function.Name] = t
}

// Remove 按名称移除 Tool,返回是否存在。
func (r *ToolRegistry) Remove(name string) bool {
	if r.tools == nil {
		return false
	}
	if _, ok := r.tools[name]; !ok {
		return false
	}
	delete(r.tools, name)
	return true
}

// Get 按名称获取 Tool;不存在时返回 (Tool, false)。
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All 返回注册表内全部 Tool,顺序不保证。
func (r *ToolRegistry) All() []Tool {
	all := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		all = append(all, t)
	}
	return all
}

// ── Tool 类型常量 ──────────────────────────────────────────────────────────

// ToolTypeFunction 目前唯一支持的 tool 类型。
const ToolTypeFunction = "function"
