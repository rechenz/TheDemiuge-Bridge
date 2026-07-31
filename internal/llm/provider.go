package llm

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// Provider 是 LLM 提供方的统一抽象。
// 上层(agent / server)只依赖本接口,不关心具体实现;
// 切换模型(DeepSeek / OpenAI / 本地模型等)时只需新增一个实现,上层零改动。
type Provider interface {
	// Chat 非流式对话入口:一次调用,直接返回完整响应
	// (含 message + tool_calls + usage)。
	// messages 为空时返回 (nil, nil)。
	Chat(ctx context.Context, messages []types.Message) (*types.ChatResponse, error)

	// ChatStream 流式对话入口:逐 chunk 回调 onChunk 推送增量,
	// 流结束后返回聚合的完整响应,错误经返回值传递。
	// 回调返回错误时立即中止。messages 为空时返回 (nil, nil)。
	ChatStream(ctx context.Context, messages []types.Message, onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error)
}
