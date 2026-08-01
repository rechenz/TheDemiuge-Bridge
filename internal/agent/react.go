// Package agent 实现 Agent 的 ReAct 循环。
//
// Runner 驱动单个 Agent 与 LLM 的多轮交互:调用 LLM → 判断工具调用
// → 执行工具回馈 → 再请求,直至模型产出最终回复。
//
// 推送策略:
//   - actor 类型的 Agent 使用 ChatStream 流式,实时把文本增量推送前端;
//   - system 类型的 Agent 使用 Chat 非流式,所有运行信息仅在调试模式下
//     经 EventSink.OnDebug 调出;
//   - ActorAgent 的评述(推理内容 / 旁白)默认不推送,写入 Result.Commentaries
//     保留,供调用方按需转发。
package agent

import (
	"context"
	"fmt"

	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ── Tool 执行抽象 ──────────────────────────────────────────────────────────

// ToolExecutor 负责执行模型发起的 tool 调用。
// 具体工具实现(如 internal/tool/)实现本接口后注入 Runner,与 ReAct 循环解耦。
type ToolExecutor interface {
	// Execute 执行一次工具调用,返回执行结果文本。
	// 该结果作为 tool 角色消息回馈给模型,供其继续推理。
	Execute(ctx context.Context, call types.ToolCall) (string, error)
}

// ── 评述 ───────────────────────────────────────────────────────────────────

// CommentaryType 区分评述的种类。
type CommentaryType string

const (
	// CommentaryReasoning 模型的推理内容(思考过程),与最终回复分离。
	CommentaryReasoning CommentaryType = "reasoning"
	// CommentaryAside Actor 的旁白 / 内心独白:
	// ReAct 中间轮次(工具调用轮)模型产出的文本,表达当前进度与想法。
	CommentaryAside CommentaryType = "aside"
	// CommentaryDebug 调试信息(预留类型)。
	// 实际调试信息经 EventSink.OnDebug 独立通道推送,与正常对话流隔离。
	CommentaryDebug CommentaryType = "debug"
)

// Commentary 一条评述。
// ActorAgent 的评述默认不推送,仅写入 Result.Commentaries 保留;
// 需要时(如内心气泡)调用方通过 EventSink.OnCommentary 自行转发。
type Commentary struct {
	Type    CommentaryType `json:"type"`
	Content string         `json:"content"`
}

// ── 前端推送 ───────────────────────────────────────────────────────────────

// EventSink 是 Agent 运行过程的事件推送通道,由 server 层实现(如 SSE)。
// 仅 actor 类型的 Agent 调用 OnText / OnToolCall 推送正常对话流;
// system 的信息只在调试模式下经 OnDebug 调出。
// OnText / OnToolCall / OnCommentary 返回错误时终止 Agent 运行(如客户端断开)。
type EventSink interface {
	// OnText 实时推送文本增量。
	// 包括旁白(ReAct 中间轮次模型说出的进度与内心想法)与最终回复,
	// 调用方无需区分,全部是角色"说"的话。
	OnText(delta string) error
	// OnToolCall 推送一次工具调用事件。
	OnToolCall(call types.ToolCall) error
	// OnCommentary 推送评述(如推理内容)。
	// 默认 handler 忽略;需要展示时转发,失败时终止运行。
	OnCommentary(c Commentary) error
	// OnDebug 推送调试信息。
	// 独立通道,便于之后直接上送前端调试面板,与正常对话流隔离。
	OnDebug(msg string) error
}

// ── 运行结果 ───────────────────────────────────────────────────────────────

// Result 一次 Run 的运行结果。
type Result struct {
	// Reply 最终正常回复
	Reply string `json:"reply"`
	// Commentaries 评述集合(推理 + 旁白),接口保留;默认不推送
	Commentaries []Commentary `json:"commentaries,omitempty"`
	// Usage token 用量
	Usage *types.Usage `json:"usage,omitempty"`
}

// ── Runner ─────────────────────────────────────────────────────────────────

// Runner 驱动单个 Agent 的 ReAct 循环。
// 一个 Runner 绑定一个 Agent(含其多会话上下文);并发安全由调用方保证。
type Runner struct {
	agent     *types.Agent
	provider  llm.Provider
	executor  ToolExecutor
	maxRounds int
	debug     bool
}

// RunnerOption 是 NewRunner 的函数式选项。
type RunnerOption func(*Runner)

// WithMaxRounds 设置 ReAct 最大轮次(默认 10),防止工具循环失控。
func WithMaxRounds(n int) RunnerOption {
	return func(r *Runner) { r.maxRounds = n }
}

// WithDebug 开启调试模式(默认关闭)。
// 开启后,system 的运行信息(推理、工具过程、usage 等)经 EventSink.OnDebug 调出。
func WithDebug(b bool) RunnerOption {
	return func(r *Runner) { r.debug = b }
}

// NewRunner 构造 Runner。
// provider 为 LLM 提供方;executor 为工具执行器,可为 nil
// (此时模型若发起工具调用,Run 返回错误)。
func NewRunner(a *types.Agent, provider llm.Provider, executor ToolExecutor, opts ...RunnerOption) *Runner {
	if a == nil {
		panic("agent: NewRunner 的 agent 不能为 nil")
	}
	if provider == nil {
		panic("agent: NewRunner 的 provider 不能为 nil")
	}
	r := &Runner{
		agent:     a,
		provider:  provider,
		executor:  executor,
		maxRounds: 10,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run 执行一次 ReAct 循环:
//  1. 追加用户消息到会话;
//  2. 循环调用 LLM(actor 流式推送 / system 非流式);
//  3. 判断 tool_calls → 执行工具回馈 → 继续请求;
//  4. 模型产出最终回复时返回 Result(含评述与 usage)。
//
// sink 可为 nil,此时跳过全部推送(actor 的文本流也随之跳过)。
func (r *Runner) Run(ctx context.Context, sessionID, userMessage string, sink EventSink) (*Result, error) {
	r.agent.AppendMessage(sessionID, types.UserMessage{Content: userMessage})

	result := &Result{}

	for round := 0; round < r.maxRounds; round++ {
		r.debugf(sink, "ReAct 第 %d 轮,会话历史 %d 条", round, len(r.agent.GetMessages(sessionID)))

		resp, err := r.callOnce(ctx, sessionID, sink)
		if err != nil {
			return nil, err
		}
		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("API 返回空 choices")
		}

		msg := resp.Choices[0].Message
		finish := resp.Choices[0].FinishReason

		// 采纳 assistant 消息(带 tool_calls 时原样回传;纯调用轮 content 置 nil)
		r.agent.AppendMessage(sessionID, normalizeAssistant(msg))

		// 评述:actor 的推理内容归档保留,并按需经 OnCommentary 通知
		if err := r.collectReasoning(sink, msg, result); err != nil {
			return nil, err
		}

		// 工具调用轮:执行工具回馈后继续下一轮
		if finish == types.FinishReasonToolCalls || len(msg.ToolCalls) > 0 {
			r.collectAside(msg, result) // 归档旁白(该轮文本)
			for _, call := range msg.ToolCalls {
				if sink != nil {
					if err := sink.OnToolCall(call); err != nil {
						return nil, err
					}
				}
				value, err := r.exec(ctx, sink, call)
				if err != nil {
					return nil, err
				}
				r.agent.AppendMessage(sessionID, types.ToolMessage{Content: value, ToolCallID: call.ID})
			}
			continue
		}

		// 非工具调用轮(如 stop / length)→ 当前文本即最终回复
		result.Reply = msg.GetContent()
		r.recordUsage(resp, result)
		r.debugf(sink, "ReAct 结束:finish=%s, usage total=%d", finish, resp.Usage.TotalTokens)
		return result, nil
	}

	return nil, fmt.Errorf("达到最大轮次 %d 仍未结束", r.maxRounds)
}

// callOnce 调用一次 LLM。
// Actor:ChatStream 流式,onChunk 实时推送文本增量;
// System:Chat 非流式,不推送文本。
func (r *Runner) callOnce(ctx context.Context, sessionID string, sink EventSink) (*types.ChatResponse, error) {
	msgs := r.buildMessages(sessionID)
	opts := r.chatOptions()

	if r.agent.Type == types.AgentTypeActor {
		resp, err := r.provider.ChatStream(ctx, msgs, opts, func(chunk *types.ChatCompletionStreamChunk) error {
			if sink == nil {
				return nil
			}
			for _, choice := range chunk.Choices {
				if delta := choice.Delta.ContentText(); delta != "" {
					if err := sink.OnText(delta); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("LLM 流式返回空响应")
		}
		return resp, nil
	}

	resp, err := r.provider.Chat(ctx, msgs, opts)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("LLM 返回空响应")
	}
	return resp, nil
}

// buildMessages 组装请求消息:system prompt + 会话历史。
func (r *Runner) buildMessages(sessionID string) []types.Message {
	history := r.agent.GetMessages(sessionID)
	msgs := make([]types.Message, 0, len(history)+1)
	if r.agent.SystemPrompt != "" {
		msgs = append(msgs, types.SystemMessage{Content: r.agent.SystemPrompt})
	}
	return append(msgs, history...)
}

// chatOptions 构造携带工具定义的 ChatOptions;无工具时返回 nil。
func (r *Runner) chatOptions() *types.ChatOptions {
	if len(r.agent.Tools) == 0 {
		return nil
	}
	return types.NewChatOptions(r.agent.Tools, nil)
}

// normalizeAssistant 规范化 assistant 消息后追加历史。
// 仅当 content 为空且携带 tool_calls 时置 nil,符合 API「仅发起工具调用
// 时 content 为 null」的规范。
func normalizeAssistant(msg types.AssistantMessage) types.AssistantMessage {
	if msg.Content != nil && *msg.Content == "" && len(msg.ToolCalls) > 0 {
		msg.Content = nil
	}
	return msg
}

// collectReasoning 归档 actor 的推理内容评述,并按需经 OnCommentary 通知。
// system 的推理信息仅在调试模式下经 OnDebug 调出,不归档。
func (r *Runner) collectReasoning(sink EventSink, msg types.AssistantMessage, result *Result) error {
	if msg.ReasoningContent == nil || *msg.ReasoningContent == "" {
		return nil
	}
	if r.agent.Type == types.AgentTypeSystem {
		r.debugf(sink, "system 推理: %s", *msg.ReasoningContent)
		return nil
	}
	c := Commentary{Type: CommentaryReasoning, Content: *msg.ReasoningContent}
	result.Commentaries = append(result.Commentaries, c)
	if sink != nil {
		return sink.OnCommentary(c)
	}
	return nil
}

// collectAside 归档 actor 的旁白:工具调用轮产出的完整文本。
// 该文本在流式过程中已实时推送,此处仅归档保留,不回发推送。
func (r *Runner) collectAside(msg types.AssistantMessage, result *Result) {
	if r.agent.Type == types.AgentTypeSystem {
		return // system 不参与旁白
	}
	if text := msg.GetContent(); text != "" {
		result.Commentaries = append(result.Commentaries, Commentary{Type: CommentaryAside, Content: text})
	}
}

// exec 执行一次工具调用,支持调试输出。
// 工具执行失败不终止 ReAct,而是作为错误结果回馈模型;仅未配置
// 工具执行器时返回错误(属于开发配置遗漏,快速失败)。
func (r *Runner) exec(ctx context.Context, sink EventSink, call types.ToolCall) (string, error) {
	if r.executor == nil {
		return "", fmt.Errorf("模型请求调用工具 %s,但未配置 ToolExecutor", call.Function.Name)
	}
	r.debugf(sink, "执行工具 %s,参数: %s", call.Function.Name, call.Function.Arguments)
	value, err := r.executor.Execute(ctx, call)
	if err != nil {
		r.debugf(sink, "工具 %s 执行失败: %v", call.Function.Name, err)
		return fmt.Sprintf("错误: 工具 %s 执行失败 - %v", call.Function.Name, err), nil
	}
	r.debugf(sink, "工具 %s 返回: %s", call.Function.Name, value)
	return value, nil
}

// recordUsage 记录 token 用量;全零时跳过。
func (r *Runner) recordUsage(resp *types.ChatResponse, result *Result) {
	u := resp.Usage
	if u.TotalTokens == 0 && u.PromptTokens == 0 && u.CompletionTokens == 0 {
		return
	}
	usage := u
	result.Usage = &usage
}

// debugf 调试模式下经 EventSink.OnDebug 输出运行信息。
// 推送失败不影响主流程(调试通道与对话流隔离)。
func (r *Runner) debugf(sink EventSink, format string, args ...any) {
	if !r.debug || sink == nil {
		return
	}
	_ = sink.OnDebug(fmt.Sprintf(format, args...))
}
