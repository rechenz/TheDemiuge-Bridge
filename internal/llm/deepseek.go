package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// chatCompletionsEndpoint DeepSeek Chat Completions 接口地址。
const chatCompletionsEndpoint = "https://api.deepseek.com/chat/completions"

// sendChat 向 DeepSeek 发送非流式 Chat Completions 请求并解析响应。
// 供非流式入口 Chat 使用。
func sendChat(ctx context.Context, req *types.DeepseekChatRequest, apiKey string) (*types.DeepseekChatResponse, error) {
	resp, err := doRequest(ctx, req, apiKey)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 DeepSeek 响应失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp.StatusCode, respBody)
	}

	var chatResp types.DeepseekChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 DeepSeek 响应失败: %w", err)
	}
	return &chatResp, nil
}

// sendChatStream 向 DeepSeek 发送流式 Chat Completions 请求,
// 逐 chunk 回调 onChunk;流结束时返回聚合的完整响应,
// 错误通过返回值传递。回调返回错误时立即中止并返回该错误。
func sendChatStream(ctx context.Context, req *types.DeepseekChatRequest, apiKey string,
	onChunk func(*types.ChatCompletionStreamChunk) error) (*types.DeepseekChatResponse, error) {

	resp, err := doRequest(ctx, req, apiKey)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("读取 DeepSeek 错误响应失败: %w", readErr)
		}
		return nil, parseAPIError(resp.StatusCode, respBody)
	}

	// 聚合结果
	aggregated := &types.DeepseekChatResponse{}
	var content, reasoning strings.Builder
	var toolCalls []*types.ToolCall // 流式 tool_calls 按 index 归位拼接

	scanner := bufio.NewScanner(resp.Body)
	// SSE 单行可能携带较长 token,默认 64KB 上限可能截断,调大
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk types.ChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("解析 DeepSeek 流式 chunk 失败: %w", err)
		}

		if onChunk != nil {
			if err := onChunk(&chunk); err != nil {
				return nil, err
			}
		}

		// 聚合元信息与 usage
		if aggregated.ID == "" {
			aggregated.ID = chunk.ID
			aggregated.Object = chunk.Object
			aggregated.Created = chunk.Created
			aggregated.Model = chunk.Model
			aggregated.SystemFingerprint = chunk.SystemFingerprint
		}
		if chunk.Usage != nil {
			aggregated.Usage = *chunk.Usage
		}

		// 聚合文本与 tool_calls
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.ContentText())
			reasoning.WriteString(choice.Delta.ReasoningContentText())
			for _, dc := range choice.Delta.ToolCalls {
				mergeToolCall(&toolCalls, dc)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 DeepSeek 流式响应失败: %w", err)
	}

	if aggregated.ID == "" && len(aggregated.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek 流式响应为空")
	}

	return buildAggregatedResponse(aggregated, content.String(), reasoning.String(), toolCalls)
}

// doRequest 构造并发送 DeepSeek Chat Completions 请求,
// 返回原始 HTTP 响应(调用方负责关闭 resp.Body)。
// 供非流式/流式共用。
func doRequest(ctx context.Context, req *types.DeepseekChatRequest, apiKey string) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化 DeepSeek 请求体失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造 DeepSeek 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求 DeepSeek API 失败: %w", err)
	}
	return resp, nil
}

// parseAPIError 将 DeepSeek 非 2xx 响应体解析为 *types.DeepSeekAPIError;
// 解析失败时返回含状态码的普通错误。
func parseAPIError(statusCode int, respBody []byte) error {
	var apiErr types.DeepSeekAPIError
	if err := json.Unmarshal(respBody, &apiErr); err != nil {
		return fmt.Errorf("DeepSeek API 返回 HTTP %d,响应体: %s", statusCode, string(respBody))
	}
	return &apiErr
}

// mergeToolCall 将
// 流式增量 tool_call
// 按 index 归位。
func mergeToolCall(toolCalls *[]*types.ToolCall, delta types.ToolCall) {
	idx := -1
	for i, call := range *toolCalls {
		if call.Index == delta.Index {
			idx = i
			break
		}
	}
	if idx == -1 {
		*toolCalls = append(*toolCalls, &types.ToolCall{Index: delta.Index})
		idx = len(*toolCalls) - 1
	}
	call := (*toolCalls)[idx]
	if delta.ID != "" {
		call.ID = delta.ID
	}
	if delta.Type != "" {
		call.Type = delta.Type
	}
	if delta.Function.Name != "" {
		call.Function.Name = delta.Function.Name
	}
	call.Function.Arguments += delta.Function.Arguments
}

// buildAggregatedResponse 组装
// 聚合后的完整响应。
func buildAggregatedResponse(agg *types.DeepseekChatResponse, content, reasoning string, toolCalls []*types.ToolCall) (*types.DeepseekChatResponse, error) {
	assistant := types.AssistantMessage{Content: &content}
	if reasoning != "" {
		r := reasoning
		assistant.ReasoningContent = &r
	}
	for _, call := range toolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, *call)
	}
	agg.Choices = []types.DeepseekChatChoice{
		{Index: 0, FinishReason: types.FinishReasonStop, Message: assistant},
	}
	return agg, nil
}
