package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ForwardRequest 转发到后端执行工具时的请求体。
type ForwardRequest struct {
	// Name 被调用的工具名称
	Name string `json:"name"`
	// Arguments 工具参数(模型生成的 JSON 对象)
	Arguments map[string]any `json:"arguments"`
}

// ForwardResponse 后端侧执行工具后的响应体。
// 两种形态:
//  1. 结构化: 200 {"result": ...} —— 有 result 字段
//  2. 自由文本: 200 "some text..." —— 无 result 字段时,把整个 body 作为结果
type ForwardResponse struct {
	// Result 结构化执行结果。
	// 可为任意 JSON 值;为空时使用原始 body 作为结果文本。
	Result any `json:"result,omitempty"`
	// raw 原始响应体(保证文本结果时返回原样)
	raw string
}

// Validate 校验响应是否携带可用的执行结果。
// 结构化 result 为空且原始 body 为空时返回错误。
func (r *ForwardResponse) Validate() error {
	if len(r.raw) == 0 {
		return fmt.Errorf("后端返回空响应")
	}
	return nil
}

// Text 返回执行结果文本。
// 结构化 result 存在时 JSON 序列化后返回;否则返回原始 body。
func (r *ForwardResponse) Text() (string, error) {
	if len(r.raw) == 0 {
		return "", fmt.Errorf("后端返回空响应")
	}
	if r.Result != nil {
		data, err := json.Marshal(r.Result)
		if err != nil {
			return "", fmt.Errorf("序列化后端结果失败: %w", err)
		}
		return string(data), nil
	}
	return r.raw, nil
}

// Client 后端工具执行转发客户端。
// 根据工具注册条目解析最终转发地址(工具 Endpoint → 实例 DefaultEndpoint
// → 全局 DefaultEndpoint),向后端发起 HTTP 调用并返回执行结果。
type Client struct {
	// DefaultEndpoint 全局默认的转发地址(配置项 UE5_DEFAULT_ENDPOINT)
	DefaultEndpoint string
	// HTTPTimeout 单次转发默认超时(默认 10s)
	HTTPTimeout time.Duration
	// HTTPClient 可选注入的 http.Client;nil 时使用默认客户端
	HTTPClient *http.Client
	// APIKey 可选注入的鉴权 key,转发时写入 X-UE5-Key header
	APIKey string
}

// Forward 转发一次工具调用。
// 实例为 nil 或解析不到转发地址时返回错误;后端不可达/超时同样返回错误。
func (c *Client) Forward(ctx context.Context, inst *Instance, tool ToolReg, args map[string]any) (*ForwardResponse, error) {
	endpoint, err := c.resolveEndpoint(inst, tool)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(ForwardRequest{Name: tool.Name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("序列化转发请求失败: %w", err)
	}

	timeout := c.HTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if tool.TimeoutMS > 0 {
		timeout = time.Duration(tool.TimeoutMS) * time.Millisecond
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("构造转发请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-UE5-Key", c.APIKey)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("转发工具 %s 到后端失败: %w", tool.Name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取后端响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(body)
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return nil, fmt.Errorf("后端返回状态 %d: %s", resp.StatusCode, msg)
	}

	// 尝试解析结构化 {"result": ...};解析失败时按原始文本处理
	fr := &ForwardResponse{raw: string(body)}
	if err := json.Unmarshal(body, &fr.Result); err != nil {
		fr.Result = nil
	}
	return fr, nil
}

// resolveEndpoint 解析工具的实际转发地址:
//  1. 工具自带的 Endpoint
//  2. 实例的 DefaultEndpoint
//  3. 全局 DefaultEndpoint
//
// 全部为空时返回错误。
func (c *Client) resolveEndpoint(inst *Instance, tool ToolReg) (string, error) {
	if tool.Endpoint != "" {
		return tool.Endpoint, nil
	}
	if inst != nil && inst.DefaultEndpoint != "" {
		return inst.DefaultEndpoint, nil
	}
	if c.DefaultEndpoint != "" {
		return c.DefaultEndpoint, nil
	}
	return "", fmt.Errorf("工具 %q 未配置转发地址(请设置 tool.Endpoint、实例 DefaultEndpoint 或全局 UE5_DEFAULT_ENDPOINT)", tool.Name)
}
