package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
)

type Agent struct {
	Registry    *ToolRegistry
	Model       string
	MaxTokens   int
	Temperature float32
}

func NewAgent(registry *ToolRegistry, model string) *Agent {
	return &Agent{
		Registry:    registry,
		Model:       model,
		MaxTokens:   2048,
		Temperature: 0.7,
	}
}

func (a *Agent) Run(ctx context.Context, apiKey string, state *State) (string, error) {
	for state.Round < state.MaxRounds {
		// 1. 将 state.Messages 转为 []llm.chatMessage
		msgs := make([]llm.ChatMessage, len(state.Messages))
		for i, m := range state.Messages {
			msgs[i] = llm.ChatMessage{
				Role:    m.Role,
				Content: m.Content,
			}
		}

		// 2. 调用 ChatCompletion（带 tools）
		resp, err := llm.ChatCompletion(ctx, apiKey, a.Model, msgs, a.Registry.ToDeepSeekTools(), a.MaxTokens, a.Temperature)
		if err != nil {
			return "", fmt.Errorf("对话返回错误:%w", err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("API 返回空 choices")
		}

		choice := resp.Choices[0]

		// 3. 追加 assistant 回复到 state
		assistantMsg := Message{
			Role:    choice.Message.Role,
			Content: choice.Message.Content,
		}
		state.Messages = append(state.Messages, assistantMsg)
		state.Round++

		// 4. 判断是否有 tool_calls
		if len(choice.Message.ToolCalls) == 0 {
			// 没有 tool_calls，直接返回最终回复
			return choice.Message.Content, nil
		}

		// 5. ReAct：执行每个 tool_call
		for _, tc := range choice.Message.ToolCalls {
			// 解析参数
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = make(map[string]interface{})
			}

			// 查找工具
			tool, ok := a.Registry.Get(tc.Function.Name)
			if !ok {
				state.Messages = append(state.Messages, Message{
					Role:    "tool",
					Content: fmt.Sprintf("错误：工具 %s 不存在", tc.Function.Name),
				})
				continue
			}

			// 执行工具
			result, err := tool.Execute(args)
			if err != nil {
				state.Messages = append(state.Messages, Message{
					Role:    "tool",
					Content: fmt.Sprintf("错误：工具执行失败 - %v", err),
				})
				continue
			}

			// 追加 tool 结果
			state.Messages = append(state.Messages, Message{
				Role:    "tool",
				Content: result,
			})
		}
		// 继续循环，将 tool 结果发给 LLM
	}

	return "", fmt.Errorf("达到最大轮次 %d 仍未结束", state.MaxRounds)
}
