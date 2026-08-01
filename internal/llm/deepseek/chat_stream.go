package deepseek

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ChatStream 流式对话入口:逐 chunk 回调 onChunk 推送增量,
// 流结束后返回聚合的完整响应,错误经返回值传递。
// 回调返回错误时立即中止。messages 为空时返回 (nil, nil)。
// opts 为单次对话可选参数(工具定义 / 单次覆盖项),nil 表示全用配置默认。
func (c *Client) ChatStream(ctx context.Context, messages []types.Message, opts *types.ChatOptions,
	onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {

	request := c.cfg.ToChatRequest(messages, opts)
	if request == nil {
		return nil, nil
	}
	// 流式入口强制开启流式输出,不受 config.Stream 影响
	request.Stream = true
	// 固定请求 usage 统计:流结束时最后一个 chunk 携带 usage
	if request.StreamOptions == nil {
		request.StreamOptions = &types.StreamOptions{IncludeUsage: true}
	}

	return c.sendChatStream(ctx, request, c.cfg.APIKey, onChunk)
}
