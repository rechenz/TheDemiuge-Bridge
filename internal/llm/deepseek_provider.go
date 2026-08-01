package llm

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/llm/deepseek"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// DeepseekProvider 是基于 DeepSeek Chat Completions API 的 Provider 实现。
// 业务代码只依赖 Provider 接口,不直接引用本类型;
// 未来切换 OpenAI / 本地模型时,新增一个实现即可,上层零改动。
type DeepseekProvider struct {
	client *deepseek.Client
}

// NewDeepseekProvider 构造 DeepseekProvider。
func NewDeepseekProvider(cfg *config.Config) *DeepseekProvider {
	return &DeepseekProvider{client: deepseek.NewClient(cfg)}
}

// Chat 非流式对话入口:一次调用,直接返回完整响应
// (含 message + tool_calls + usage)。
// messages 为空时返回 (nil, nil)。
// opts 为单次对话可选参数(工具定义 / 单次覆盖项),nil 表示全部使用默认配置。
func (p *DeepseekProvider) Chat(ctx context.Context, messages []types.Message, opts *types.ChatOptions) (*types.ChatResponse, error) {
	return p.client.Chat(ctx, messages, opts)
}

// ChatStream 流式对话入口:逐 chunk 回调 onChunk 推送增量,
// 流结束后返回聚合的完整响应,错误经返回值传递。
// 回调返回错误时立即中止。messages 为空时返回 (nil, nil)。
// opts 为单次对话可选参数(工具定义 / 单次覆盖项),nil 表示全部使用默认配置。
func (p *DeepseekProvider) ChatStream(ctx context.Context, messages []types.Message, opts *types.ChatOptions, onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {
	return p.client.ChatStream(ctx, messages, opts, onChunk)
}
