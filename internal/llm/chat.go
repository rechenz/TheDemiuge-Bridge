package llm

import (
	"context"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// Chat 非流式对话入口:一次调用,直接返回完整响应
// (含 message + tool_calls + usage)。
// messages 为空时返回 (nil, nil),由调用方保证至少 1 条消息。
func Chat(ctx context.Context, messages []types.Message, cfg *config.Config) (*types.DeepseekChatResponse, error) {
	request := cfg.ToDeepseekRequest(messages)
	if request == nil {
		return nil, nil
	}
	// 非流式入口强制关闭流式输出,
	// 即使 config.Stream 为 true(如 STREAM 环境变量)也不影响。
	request.Stream = false
	request.StreamOptions = nil

	return sendChat(ctx, request, cfg.DeepSeekKey)
}
