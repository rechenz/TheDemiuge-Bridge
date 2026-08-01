package types

// ChatOptions 单次对话的可选参数，挂在 Provider.Chat / ChatStream 签名上。
// 传 nil 表示全部使用 config 默认值。
//
// Tools / ToolChoice 为 config 未覆盖的新维度：工具定义由调用方（agent 层）
// 按 NPC 能力动态组装，与 config 承载的通用默认参数互不冲突。
// 其余字段为单次覆盖项：非零值会覆盖 config 对应配置。
type ChatOptions struct {
	// Tools 模型可能会调用的 tool 列表，目前仅支持 function，最多 128 个。
	Tools []Tool

	// ToolChoice 控制模型调用 tool 的行为：none / auto / required / 指定函数。
	// 仅在 Tools 非空时有意义；nil 表示不发送，由服务端默认(auto)。
	ToolChoice *ToolChoice

	// Model 单次覆盖模型名；空串表示使用 config.ModelName。
	Model string

	// MaxTokens 单次覆盖最大生成 token 数；nil 表示使用 config.MaxTokens。
	MaxTokens *int

	// Temperature 单次覆盖采样温度；nil 表示使用 config.Temperature。
	Temperature *float32

	// TopP 单次覆盖核采样；nil 表示使用 config.TopP。
	TopP *float32
}

// NewChatOptions 构造带工具定义的 ChatOptions。
func NewChatOptions(tools []Tool, toolChoice *ToolChoice) *ChatOptions {
	return &ChatOptions{Tools: tools, ToolChoice: toolChoice}
}
