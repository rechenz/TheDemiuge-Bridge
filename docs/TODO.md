# TheDemiuge-Bridge 任务追踪

> 最后更新：2026-07-31

---

## P0 — 核心对话跑通 🎯（当前阶段）

### ✅ 已完成
- [x] `internal/types/` 完整重写 — message.go（Message 接口 + 四类消息）+ deepseek.go（请求/响应/SSE/Usage/错误）+ tools.go（Tool/JSONSchema/ToolChoice）
- [x] `internal/config/` 重写 — 环境变量加载 + `ToDeepseekRequest()` 映射
- [x] `go build ./...` 通过
- [x] 提交 types/config 改动（7108a5b）
- [x] 文档纳入版本管理（61661b9，docs/ARCHITECTURE.md + docs/TODO.md）

### 📌 待办
- [ ] LLM 客户端重写 `internal/llm/`（旧 deepseek.go/completion.go 已删）
  - [ ] `client.go` — 统一入口 `Chat(ctx, req) → <-chan Token`（流式/非流式同一接口）
  - [ ] `deepseek.go` — 用 types.DeepseekChatRequest / ChatCompletionStreamChunk 实现
  - [ ] `provider.go` — Provider 接口抽象
  - [ ] 错误处理：DeepSeekAPIError → 重试/降级策略
- [ ] Agent 层重建 `internal/agent/`（已删，需重建）
  - [ ] `agent.go` — Run 循环（LLM → tool_calls → 执行 → 回馈 → 再请求）
  - [ ] `builder.go` — NPC 角色 prompt 组装
- [ ] Tool 系统 `internal/tool/`
  - [ ] `registry.go` — 注册中心
  - [ ] `tools/time.go` — 示例工具（从旧 example_tool.go 迁移）
- [ ] Server 层重建 `internal/server/`
  - [ ] `handler/chat.go` — POST /api/chat（流式 SSE）
  - [ ] 路由注册 + `cmd/server/main.go` 接线
- [ ] 验证：curl → HTTP SSE → DeepSeek → tool call → 返回
- [ ] 提交 P0 完成版本

### 🧹 清理
- [ ] README.md 更新（当前只有一行）

---

## P1 — NPC 管理

- [ ] `store/npc.go` — NPCStore 接口 + mem 实现
- [ ] NPC profile 可配置（JSON/YAML 加载）
- [ ] 多 NPC 并发会话
- [ ] 会话超时/自动清理

## P2 — 记忆系统

- [ ] 短期记忆上下文窗口
- [ ] 长期记忆持久化
- [ ] 记忆召回 → prompt 注入

## P3 — 引擎对接

- [ ] 协议定型
- [ ] UE5 C++ 插件

## P4 — 异步蒸馏

- [ ] 轻量模型集成
- [ ] 后台蒸馏任务
- [ ] LoRA weight 管理
