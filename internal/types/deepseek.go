// Package types 定义了 DeepSeek Chat Completions API 的完整类型体系,
// 覆盖官方文档 https://api-docs.deepseek.com/zh-cn/api/create-chat-completion/
// 的全部请求参数、非流式响应、流式响应(SSE)、token 用量与对数概率结构。
package types

import (
	"encoding/json"
	"fmt"
)

// ── 模型 ID ────────────────────────────────────────────────────────────────

const (
	// ModelV4Flash 快速模型,支持 low/high/max 三档推理强度
	ModelV4Flash = "deepseek-v4-flash"
	// ModelV4Pro 旗舰模型,目前支持 high/max 两档推理强度
	ModelV4Pro = "deepseek-v4-pro"
)

// ── 响应对象类型 ───────────────────────────────────────────────────────────

const (
	// ObjectChatCompletion 非流式响应对象类型
	ObjectChatCompletion = "chat.completion"
	// ObjectChatCompletionChunk 流式响应 chunk 对象类型
	ObjectChatCompletionChunk = "chat.completion.chunk"
)

// ── 停止原因 ───────────────────────────────────────────────────────────────

// FinishReason 描述模型停止生成 token 的原因。
type FinishReason string

const (
	// FinishReasonStop 模型自然停止生成,或遇到 stop 序列中列出的字符串
	FinishReasonStop FinishReason = "stop"
	// FinishReasonLength 输出长度达到模型上下文长度限制或 max_tokens 限制
	FinishReasonLength FinishReason = "length"
	// FinishReasonContentFilter 输出内容因触发过滤策略而被过滤
	FinishReasonContentFilter FinishReason = "content_filter"
	// FinishReasonToolCalls 模型调用了 function
	FinishReasonToolCalls FinishReason = "tool_calls"
	// FinishReasonInsufficientSystemResource 系统推理资源不足,生成被打断
	FinishReasonInsufficientSystemResource FinishReason = "insufficient_system_resource"
)

// ── 思考模式 ───────────────────────────────────────────────────────────────

// ThinkingConfig 控制思考模式与非思考模式的转换。
// 对应请求体顶层字段 thinking,nullable。
type ThinkingConfig struct {
	// Type 取值为 enabled(思考模式)或 disabled(非思考模式),默认 enabled
	Type string `json:"type"`
}

const (
	// ThinkingEnabled 使用思考模式
	ThinkingEnabled = "enabled"
	// ThinkingDisabled 使用非思考模式
	ThinkingDisabled = "disabled"
)

// ── 输出格式 ───────────────────────────────────────────────────────────────

// ResponseFormat 指定模型必须输出的格式。
// 对应请求体顶层字段 response_format,nullable。
type ResponseFormat struct {
	// Type 取值为 text 或 json_object,默认 text
	Type string `json:"type"`
}

const (
	// ResponseFormatText 普通文本输出
	ResponseFormatText = "text"
	// ResponseFormatJSONObject 启用 JSON 模式,保证模型输出有效 JSON。
	// 注意:使用时必须在 system/user 消息中指示模型生成 JSON。
	ResponseFormatJSONObject = "json_object"
)

// ── 流式选项 ───────────────────────────────────────────────────────────────

// StreamOptions 流式输出相关选项,仅在 stream 为 true 时可设置。
type StreamOptions struct {
	// IncludeUsage 为 true 时,在流式消息最后的 data: [DONE] 之前传输一个额外块,
	// 该块 usage 字段包含整次请求的 token 统计,choices 为空数组;
	// 其余块 usage 为 null。
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ── 停止序列 ───────────────────────────────────────────────────────────────

// StopSequence 是停止序列,兼容文档中 string 与最多 16 个 string 的数组两种形式。
type StopSequence struct {
	Values []string
}

// NewStopSequence 构造单值停止序列。
func NewStopSequence(value string) *StopSequence {
	return &StopSequence{Values: []string{value}}
}

// NewStopSequenceList 构造多值停止序列(最多 16 个,超出截断)。
func NewStopSequenceList(values ...string) *StopSequence {
	if len(values) > 16 {
		values = values[:16]
	}
	return &StopSequence{Values: values}
}

// UnmarshalJSON 支持 string 或 []string 两种输入形式。
func (s *StopSequence) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		s.Values = []string{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("stop 必须是 string 或 string 数组: %w", err)
	}
	s.Values = list
	return nil
}

// MarshalJSON 单值时输出字符串,多值时输出数组。
func (s StopSequence) MarshalJSON() ([]byte, error) {
	if s.Values == nil {
		return []byte("null"), nil
	}
	if len(s.Values) == 1 {
		return json.Marshal(s.Values[0])
	}
	return json.Marshal(s.Values)
}

// ── 请求体 ─────────────────────────────────────────────────────────────────

// DeepseekChatRequest 对应 POST /chat/completions 的请求体。
// 可选字段均带 omitempty,nil 值不会发送,由服务端使用文档默认值。
type DeepseekChatRequest struct {
	// Messages 对话的消息列表,至少 1 条(必填)
	Messages []Message `json:"messages"`
	// Model 使用的模型 ID:deepseek-v4-flash 或 deepseek-v4-pro(必填)
	Model string `json:"model"`

	// Thinking 控制思考模式;nil 表示不发送,由服务端默认(enabled)
	Thinking *ThinkingConfig `json:"thinking,omitempty"`
	// ReasoningEffort 推理强度:low / high / max,默认 high;
	// 兼容值 medium、xhigh 会映射为 high
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// MaxTokens 限制一次请求中模型生成 completion 的最大 token 数
	MaxTokens *int `json:"max_tokens,omitempty"`
	// ResponseFormat 指定模型必须输出的格式(text 或 json_object)
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	// Stop 遇到这些词时 API 停止生成更多 token;string 或最多 16 个 string 的数组
	Stop *StopSequence `json:"stop,omitempty"`
	// Stream 为 true 时以 SSE 流式发送增量,消息流以 data: [DONE] 结尾
	Stream bool `json:"stream,omitempty"`
	// StreamOptions 流式输出相关选项,仅在 Stream 为 true 时可设置
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	// Temperature 采样温度,介于 0 和 2 之间,默认 1。
	// 建议与 TopP 二选一进行修改
	Temperature *float32 `json:"temperature,omitempty"`
	// TopP 核采样,介于 0 和 1 之间,默认 1。
	// 建议与 Temperature 二选一进行修改
	TopP *float32 `json:"top_p,omitempty"`

	// Tools 模型可能会调用的 tool 列表,目前仅支持 function,最多 128 个
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice 控制模型调用 tool 的行为:
	// none / auto / required / 指定调用某个函数
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`

	// Logprobs 是否返回所输出 token 的对数概率
	Logprobs *bool `json:"logprobs,omitempty"`
	// TopLogprobs 0~20 的整数 N,返回每个输出位置概率 top N 的 token 对数概率;
	// 指定时 Logprobs 必须为 true
	TopLogprobs *int `json:"top_logprobs,omitempty"`
	// UserID 自定义业务侧用户标识,可选字符集 [a-zA-Z0-9\-_],最大长度 512;
	// 用于内容安全、KVCache 缓存隔离与调度隔离
	UserID string `json:"user_id,omitempty"`
}

// ── 非流式响应 ─────────────────────────────────────────────────────────────

// DeepseekChatResponse 对应非流式 200 响应体。
type DeepseekChatResponse struct {
	// ID 该对话的唯一标识符
	ID string `json:"id"`
	// Object 固定为 chat.completion
	Object string `json:"object"`
	// Created 创建聊天完成时的 Unix 时间戳(以秒为单位)
	Created int64 `json:"created"`
	// Model 生成该 completion 的模型名
	Model string `json:"model"`
	// Choices 模型生成的 completion 的选择列表
	Choices []DeepseekChatChoice `json:"choices"`
	// SystemFingerprint 表示模型运行的后端配置指纹
	SystemFingerprint string `json:"system_fingerprint"`
	// Usage 该对话补全请求的用量信息
	Usage Usage `json:"usage"`
}

// DeepseekChatChoice 单个 completion 选择。
type DeepseekChatChoice struct {
	// Index 该 completion 在选择列表中的索引
	Index int `json:"index"`
	// FinishReason 模型停止生成 token 的原因
	FinishReason FinishReason `json:"finish_reason"`
	// Message 模型生成的 completion 消息
	Message AssistantMessage `json:"message"`
	// Logprobs 该 choice 的对数概率信息;未开启 logprobs 时为 null
	Logprobs *Logprobs `json:"logprobs"`
}

// ── 流式响应(SSE chunk)───────────────────────────────────────────────────

// ChatCompletionStreamChunk 对应流式响应中的单个 SSE 数据块(data: 后面的 JSON)。
type ChatCompletionStreamChunk struct {
	// ID 该对话的唯一标识符;流式响应的每个 chunk 的 ID 相同
	ID string `json:"id"`
	// Object 固定为 chat.completion.chunk
	Object string `json:"object"`
	// Created 创建聊天完成时的 Unix 时间戳(秒);流式每个 chunk 相同
	Created int64 `json:"created"`
	// Model 生成该 completion 的模型名
	Model string `json:"model"`
	// Choices 增量选择列表
	Choices []ChatCompletionStreamChoice `json:"choices"`
	// SystemFingerprint 模型运行的后端配置指纹
	SystemFingerprint string `json:"system_fingerprint"`
	// Usage 仅在 stream_options.include_usage 为 true 时,
	// 最后一个 chunk 携带整次请求的 token 统计,其余 chunk 为 null
	Usage *Usage `json:"usage"`
}

// ChatCompletionStreamChoice 流式 chunk 中的单个增量选择。
type ChatCompletionStreamChoice struct {
	// Index 该 completion 在选择列表中的索引
	Index int `json:"index"`
	// Delta 消息的增量内容
	Delta Delta `json:"delta"`
	// FinishReason 结束原因;未结束时为 null(解码为空字符串)
	FinishReason FinishReason `json:"finish_reason"`
	// Logprobs 该 choice 的对数概率信息
	Logprobs *Logprobs `json:"logprobs"`
}

// Delta 流式增量消息内容。
type Delta struct {
	// Role 首个出现的 chunk 中为 assistant,后续为 null(解码为空字符串)
	Role string `json:"role,omitempty"`
	// Content 增量文本内容;为 null 时为零值 nil
	Content *string `json:"content"`
	// ReasoningContent 思考模式下最终答案之前的增量推理内容
	ReasoningContent *string `json:"reasoning_content,omitempty"`
	// ToolCalls 增量 function 调用信息
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ContentText 返回增量文本内容;content 为 null 时返回空串。
func (d Delta) ContentText() string {
	if d.Content == nil {
		return ""
	}
	return *d.Content
}

// ReasoningContentText 返回增量推理内容;为 null 时返回空串。
func (d Delta) ReasoningContentText() string {
	if d.ReasoningContent == nil {
		return ""
	}
	return *d.ReasoningContent
}

// ── Token 用量 ────────────────────────────────────────────────────────────

// Usage 一次对话补全请求的 token 用量统计。
type Usage struct {
	// CompletionTokens 模型 completion 产生的 token 数
	CompletionTokens int `json:"completion_tokens"`
	// PromptTokens 用户 prompt 所包含的 token 数,
	// 等于 PromptCacheHitTokens + PromptCacheMissTokens
	PromptTokens int `json:"prompt_tokens"`
	// PromptCacheHitTokens 用户 prompt 中命中上下文缓存的 token 数
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	// PromptCacheMissTokens 用户 prompt 中未命中上下文缓存的 token 数
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
	// TotalTokens 该请求中所有 token 的数量(prompt + completion)
	TotalTokens int `json:"total_tokens"`
	// CompletionTokensDetails completion tokens 的详细信息
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// CompletionTokensDetails completion tokens 的详细信息。
type CompletionTokensDetails struct {
	// ReasoningTokens 用于 chain of thought 的 token 数
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ── 对数概率 ──────────────────────────────────────────────────────────────

// Logprobs 输出 token 的对数概率信息。
type Logprobs struct {
	// Content 包含输出 content token 对数概率信息的列表
	Content []LogprobsContent `json:"content"`
	// ReasoningContent 思考模式下输出 reasoning_content token 的对数概率列表
	ReasoningContent []LogprobsContent `json:"reasoning_content,omitempty"`
}

// LogprobsContent 单个输出 token 的对数概率详情。
type LogprobsContent struct {
	// Token 输出的 token
	Token string `json:"token"`
	// Logprob 该 token 的对数概率;
	// -9999.0 表示概率极小,不在 top 20 最可能输出的 token 中
	Logprob float64 `json:"logprob"`
	// Bytes 该 token UTF-8 字节表示的整数列表;
	// 一般在一个 UTF-8 字符被拆分成多个 token 时有用;无字节表示时为 null
	Bytes []int `json:"bytes"`
	// TopLogprobs 该输出位置上输出概率 top N 的 token 列表及其对数概率
	TopLogprobs []TopLogprob `json:"top_logprobs"`
}

// TopLogprob 一个候选 token 的对数概率。
type TopLogprob struct {
	// Token 候选 token
	Token string `json:"token"`
	// Logprob 该 token 的对数概率
	Logprob float64 `json:"logprob"`
	// Bytes 该 token UTF-8 字节表示的整数列表
	Bytes []int `json:"bytes"`
}

// ── 错误响应 ──────────────────────────────────────────────────────────────

// DeepSeekAPIError 对应 API 错误响应体 {"error": {...}}。
// 便于 Agent 层根据 message / type / code 判断错误并决定重试或降级策略。
type DeepSeekAPIError struct {
	Err ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情。
type ErrorDetail struct {
	// Message 错误描述信息
	Message string `json:"message"`
	// Type 错误类型
	Type string `json:"type"`
	// Code 错误码
	Code string `json:"code"`
}

// Error 实现 error 接口。
func (e *DeepSeekAPIError) Error() string {
	return fmt.Sprintf("DeepSeek API 错误(type=%s, code=%s): %s", e.Err.Type, e.Err.Code, e.Err.Message)
}
