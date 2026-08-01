package deepseek

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// chatCompletionsEndpoint DeepSeek Chat Completions 接口地址。
// 仅作为兜底,优先使用 config.BaseURL。
const chatCompletionsEndpoint = config.DeepSeekBaseURL

// ── 错误分类哨兵 ────────────────────────────────────────────────────────────
//
// 供上层(agent 层)用 errors.Is(err, deepseek.ErrXxx) 判断错误类别,
// 决定立即失败 / 退避重试 / 降级等策略。

var (
	// ErrAuth 认证失败(401 / 403):API Key 无效或无权限。
	ErrAuth = errors.New("deepseek: 认证失败(401/403)")
	// ErrRateLimit 触发限流(429):请求过于频繁。
	ErrRateLimit = errors.New("deepseek: 请求限流(429)")
	// ErrServer 服务端错误(5xx):可重试。
	ErrServer = errors.New("deepseek: 服务端错误(5xx)")
	// ErrTimeout 请求超时(上下文取消或截止时间到达)。
	ErrTimeout = errors.New("deepseek: 请求超时")
)

// ClassifiedError 带分类的 API 错误。
// Unwrap 返回分类哨兵,使 errors.Is(err, ErrRateLimit) 等判断生效;
// API 字段保留原始错误详情。
type ClassifiedError struct {
	API  *types.APIError
	Kind error
}

// Error 实现 error 接口。
func (e *ClassifiedError) Error() string {
	return e.API.Error()
}

// Unwrap 返回分类哨兵,支持 errors.Is / errors.As 链式判断。
func (e *ClassifiedError) Unwrap() error {
	return e.Kind
}

// Client 是 DeepSeek Chat Completions 的客户端。
// 持有配置与可注入的 http.Client,所有请求经它发出。
type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewClient 构造 DeepSeek 客户端。
// cfg.HTTPClient 非 nil 时优先使用注入的客户端;
// 否则使用带 cfg.HTTPTimeout 超时的默认客户端。
func NewClient(cfg *config.Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil && cfg.HTTPTimeout > 0 {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &Client{cfg: cfg, httpClient: httpClient}
}

// sendChat 向 DeepSeek 发送非流式 Chat Completions 请求并解析响应。
// 供非流式入口 Chat 使用。
func (c *Client) sendChat(ctx context.Context, req *types.ChatRequest, apiKey string) (*types.ChatResponse, error) {
	resp, err := c.doRequest(ctx, req, apiKey)
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

	var chatResp types.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 DeepSeek 响应失败: %w", err)
	}
	return &chatResp, nil
}

// sendChatStream 向 DeepSeek 发送流式 Chat Completions 请求,
// 逐 chunk 回调 onChunk;流结束时返回聚合的完整响应,
// 错误通过返回值传递。回调返回错误时立即中止并返回该错误。
func (c *Client) sendChatStream(ctx context.Context, req *types.ChatRequest, apiKey string,
	onChunk func(*types.ChatCompletionStreamChunk) error) (*types.ChatResponse, error) {

	resp, err := c.doRequest(ctx, req, apiKey)
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
	aggregated := &types.ChatResponse{}
	var content, reasoning strings.Builder
	var toolCalls []*types.ToolCall // 流式 tool_calls 按 index 归位拼接
	// 按 choice index 收集最后出现的 finish_reason(usage chunk 的 choices 为空,不影响)
	finishReasons := map[int]types.FinishReason{}

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

		// 聚合文本、tool_calls 与真实 finish_reason
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.ContentText())
			reasoning.WriteString(choice.Delta.ReasoningContentText())
			for _, dc := range choice.Delta.ToolCalls {
				mergeToolCall(&toolCalls, dc)
			}
			if choice.FinishReason != "" {
				finishReasons[choice.Index] = choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 DeepSeek 流式响应失败: %w", err)
	}

	if aggregated.ID == "" && len(aggregated.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek 流式响应为空")
	}

	// 取 index 0 的真实结束原因;缺失时兜底 stop
	finishReason := finishReasons[0]
	if finishReason == "" {
		finishReason = types.FinishReasonStop
	}

	return buildAggregatedResponse(aggregated, content.String(), reasoning.String(), toolCalls, finishReason)
}

// doRequest 构造并发送 DeepSeek Chat Completions 请求,
// 返回原始 HTTP 响应(调用方负责关闭 resp.Body)。
// 供非流式/流式共用。
func (c *Client) doRequest(ctx context.Context, req *types.ChatRequest, apiKey string) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化 DeepSeek 请求体失败: %w", err)
	}

	baseURL := c.cfg.BaseURL
	if baseURL == "" {
		baseURL = chatCompletionsEndpoint
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造 DeepSeek 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		// 区分超时类错误:上下文截止 / 客户端 Timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("请求 DeepSeek API 失败: %w", err)
	}
	return resp, nil
}

// parseAPIError 将 DeepSeek 非 2xx 响应体解析为 *ClassifiedError;
// 无法解析错误体时返回含状态码的普通错误。
// 分类哨兵(ErrAuth / ErrRateLimit / ErrServer)供 errors.Is 判断。
func parseAPIError(statusCode int, respBody []byte) error {
	var apiErr types.APIError
	if err := json.Unmarshal(respBody, &apiErr); err != nil || apiErr.Err.Message == "" {
		return fmt.Errorf("DeepSeek API 返回 HTTP %d,响应体: %s", statusCode, string(respBody))
	}
	return &ClassifiedError{API: &apiErr, Kind: classifyStatus(statusCode)}
}

// classifyStatus 将 HTTP 状态码映射为错误分类哨兵。
func classifyStatus(statusCode int) error {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return ErrAuth
	case statusCode == http.StatusTooManyRequests:
		return ErrRateLimit
	case statusCode >= 500:
		return ErrServer
	default:
		return nil
	}
}

// mergeToolCall 将流式增量 tool_call 按 index 归位。
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

// buildAggregatedResponse 组装聚合后的完整响应。
// finishReason 为流式循环收集到的真实结束原因,而非硬编码 stop。
func buildAggregatedResponse(agg *types.ChatResponse, content, reasoning string, toolCalls []*types.ToolCall, finishReason types.FinishReason) (*types.ChatResponse, error) {
	assistant := types.AssistantMessage{Content: &content}
	if reasoning != "" {
		r := reasoning
		assistant.ReasoningContent = &r
	}
	for _, call := range toolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, *call)
	}
	agg.Choices = []types.ChatChoice{
		{Index: 0, FinishReason: finishReason, Message: assistant},
	}
	return agg, nil
}
