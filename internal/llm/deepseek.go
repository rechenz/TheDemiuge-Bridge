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

	"github.com/rechenz/TheDemiuge-Bridge/internal/agent"
)

// 发给 DeepSeek API 的请求结构
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func ChatStream(ctx context.Context, apiKey, model string, messages []agent.Message, maxTokens int, temperature float32, onToken func(string)) (string, error) {
	// ── 1. 把 agent.Message 转成 chatMessage ──
	// TODO: 遍历 messages，逐个转换
	var cM []chatMessage
	for _, v := range messages {
		cM = append(cM, chatMessage{
			Role:    v.Role,
			Content: v.Content,
		})
	}
	// ── 2. 构建请求体 ──
	// TODO: 创建 chatRequest，Stream 设为 true
	var cR chatRequest
	cR.Stream = true
	cR.Model = model
	cR.Temperature = temperature
	cR.MaxTokens = maxTokens
	cR.Messages = cM
	// ── 3. 序列化 JSON ──
	// TODO: json.Marshal
	js, err := json.Marshal(cR)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}
	// ── 4. 创建 HTTP POST 请求 ──
	// TODO: http.NewRequestWithContext + 设置 Header (Content-Type, Authorization, Accept)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.deepseek.com/chat/completions", bytes.NewReader(js))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	// ── 5. 发送请求 ──
	// TODO: http.DefaultClient.Do
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	// ── 6. 检查状态码 ──
	// TODO: resp.StatusCode != 200 → 读 body 返回错误
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(body))
	}
	// ── 7. 逐行解析 SSE ──
	// TODO: bufio.NewScanner 循环, strings.HasPrefix("data: "), 解析 chatStreamChunk
	//       拼完整文本, 有 onToken 就回调
	//       遇到 "data: [DONE]" 或 finish_reason=="stop" 就 break
	scanner := bufio.NewScanner(resp.Body)
	var fullText strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if line == "data: [DONE]" {
			break
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			token := choice.Delta.Content
			if token != "" {
				fullText.WriteString(token)
				if onToken != nil {
					onToken(token)
				}
			}

			if choice.FinishReason == "stop" {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取流失败: %w", err)
	}
	// ── 8. 返回完整文本 ──
	return fullText.String(), nil
}
