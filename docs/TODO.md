# TheDemiuge-Bridge 任务追踪

> 最后更新：2026-08-01（LLM 层全量完善并提交）

---

## P0 — 核心对话跑通 🎯（当前阶段）

### ✅ 已完成

- [X] `internal/types/` 完整重写 — message.go（Message 接口 + 四类消息）+ deepseek.go（请求/响应/SSE/Usage/错误）+ tools.go（Tool/JSONSchema/ToolChoice）
- [X] `internal/config/` 重写 — 环境变量加载 + `ToDeepseekRequest()` 映射
- [X] `go build ./...` 通过
- [X] 提交 types/config 改动（7108a5b）
- [X] 文档纳入版本管理（61661b9，docs/ARCHITECTURE.md + docs/TODO.md）
- [X] 架构评审完成（2026-07-31）：回调式 LLM 入口 / schema 反射 / SSE 批量 / service 延后 / memory 三路线

### 📌 待办

- [X] LLM 客户端重写 `internal/llm/`（旧 deepseek.go/completion.go 已删）
  - [X] **回调式两入口**：`Chat(ctx, req) (*ChatResponse, error)` 非流式 + `ChatStream(ctx, req, onToken)` 流式
  - [X] tool_calls 增量在客户端内部拼接，流结束随响应返回，不混在 Token 里
  - [X] `provider.go` — Provider 接口抽象
  - [X] 错误处理：DeepSeekAPIError → 重试/降级策略
  - [X] **2026-08-01 全量完善**：Tools 通道（ChatOptions）+ finish_reason 保留 + BaseURL 可配置 + http.Client 可注入 + 错误分类哨兵 + httptest 单测
- [ ] Agent 层重建 `internal/agent/`（已删，需重建）
  - [ ] `agent.go` — Run 循环（LLM → 流结束判断 tool_calls → 执行 → 回馈 → 再请求）
  - [ ] `builder.go` — NPC 角色 prompt 组装
- [ ] Tool 系统 `internal/tool/`
  - [ ] `registry.go` — 注册中心
  - [ ] `FromStruct(v any)` — struct + tag → JSON Schema 反射工具（tag: required/default/enum）
  - [ ] `tools/time.go` — 示例工具（用 FromStruct 定义参数）
- [ ] Server 层重建 `internal/server/`（P0 不设 service 层，handler 直连 agent）
  - [ ] `handler/chat.go` — POST /api/chat（SSE 批量推送 + error 事件）
  - [ ] X-API-Key 鉴权（API_KEY 配置，空则仅 localhost）
  - [ ] 路由注册 + `cmd/server/main.go` 接线
- [ ] 验证：curl → HTTP SSE → DeepSeek → tool call → 返回
- [ ] 提交 P0 完成版本

### 🧹 清理

- [ ] README.md 更新（当前只有一行）

---

## P1 — NPC 管理

- [ ] 拆出轻量 `service/` 层（会话生命周期 + NPC 编排）
- [ ] `store/npc.go` — NPCStore 接口 + mem 实现
- [ ] NPC profile 可配置（JSON/YAML 加载）
- [ ] 多 NPC 并发会话
- [ ] 会话超时/自动清理

## P2 — 记忆系统（RAG 路线）

- [ ] 短期记忆上下文窗口
- [ ] 长期记忆摘要 + 事实库召回 → 注入 prompt
- [ ] MemoryStore 接口

## P3 — 引擎对接

- [ ] 协议定型（批量 SSE + error + 鉴权已定稿）
- [ ] UE5 C++ 插件

## P4 — 异步蒸馏（独立远期路线，与 weight-masking 实验联动）

- [ ] 轻量模型集成
- [ ] 后台蒸馏任务
- [ ] LoRA weight 管理

## 📌 学习方向（不阻塞主线）

- 微服务化：store 接口隔离已预留，P0~P2 不做，后续拆 gRPC 客户端练手
