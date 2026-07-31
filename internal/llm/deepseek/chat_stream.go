package deepseek

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ChatStream 流式对话入口:逐 chunk 回调 onChunk 推送增量,
// 流结束后返回聚合的完整响应,错误经返回值传递。
// 回调返回错误时立即中止。messages 为空时返回 (nil, nil)。
func ChatStream(ctx context.Context, messages []types.Message, cfg *config.Config,
	onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {

	request := cfg.ToChatRequest(messages)
	if request == nil {
		return nil, nil
	}
	// 流式入口强制开启流式输出,不受 config.Stream 影响
	request.Stream = true
	// 固定请求 usage 统计:流结束时最后一个 chunk 携带 usage
	if request.StreamOptions == nil {
		request.StreamOptions = &types.StreamOptions{IncludeUsage: true}
	}

	return sendChatStream(ctx, request, cfg.APIKey, onChunk)
}
