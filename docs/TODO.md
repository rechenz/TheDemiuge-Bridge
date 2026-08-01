# TheDemiuge-Bridge 任务追踪

> 最后更新：2026-08-01（17:20，MCP 后端解耦完成；命名 backend 化；MCP Server 规划迁往 UE5 端）

---

## P0 — 核心对话跑通 ✅（2026-08-01 完成）

### 已完成

- [X] `internal/types/` 完整重写 — message.go + deepseek.go + tools.go（提交 7108a5b）
- [X] `internal/config/` 重写 — 环境变量加载 + `ToDeepseekRequest()` 映射
- [X] `internal/llm/` — 回调式两入口（Chat / ChatStream）、tool_calls 增量拼接、Provider 抽象、错误分类哨兵、httptest 单测（提交 3cfc325 前工作区完成）
- [X] `internal/agent/react.go` — ReAct 循环（Runner）：LLM → tool_calls → 执行 → 回馈 → 再请求；actor 流式 / system 非流式；评述归档；最大轮次保护
- [X] `internal/ue5/` → `internal/backend/` — 实例注册中心（Manager + Instance + Client）：agent/tool 动态注册、落盘恢复、工具转发、变更广播（2160ec9 更名）
- [X] `internal/mcp/` — MCP 协议层（JSON-RPC 分发 + SSE Hub + **Registry 接口** + tools/prompts 方法）
- [X] `internal/server/handler/` — UE5 管理 API（X-UE5-Key 鉴权）+ MCP 入口
- [X] **`internal/mcp/registry.go` + `internal/backend/registry_adapter.go` — 工具执行走 mcp.Registry.ExecuteTool**（62a0a2a，取代 tool/UE5Executor）
- [X] **`internal/server/handler/chat.go` — POST /api/chat SSE**：text / tool_call / commentary / done / error 事件；X-API-Key 鉴权（CHAT_API_KEY）；refreshRunner 热更新
- [X] **`cmd/server/main.go` 接线**：DeepSeek Provider + RegistryAdapter + ChatHandler + MCP 共存
- [X] 单测：agent ReAct 循环（工具轮/单轮/最大轮次/无 executor）、chat 集成测试（真实 Hertz + mock UE5 + mock LLM 全链路）、chat 热更新、历史窗口裁剪、Hub 竞态、超时兜底、消息 role 序列化、实例 ID 校验
- [X] `go build ./...` / `go vet ./...` / `go test ./...` 全绿
- [X] 文档纳入版本管理（docs/ARCHITECTURE.md + docs/TODO.md）
- [X] **架构重构（62a0a2a）**：MCP 层与后端解耦（Registry 接口）；删除 internal/tool、internal/registry、config/*.yaml、RegistryConfig、ToolRegistry/AgentRegistry
- [X] **命名统一（2160ec9）**：internal/ue5 → internal/backend（保留对外业务命名 ue5_handler.go / /api/v1/ue5 / X-UE5-Key）
- [X] 历史窗口裁剪（275aa90）：maxHistoryMessages=30 + tool 配对保护

### 遗留（P0 收尾）

- [ ] 端到端联调：真实 DEEPSEEK_API_KEY + UE5 mock 跑一遍 curl → SSE

---

## P1 — MCP 迁往 UE5 端 🎯（2026-08-01 决策）

> 架构决策：**MCP Server 最终放在 UE5 端**（游戏实例进程内），Bridge 专注 ReAct 对话。
> Go 侧 `internal/mcp/` 保留作协议参考 / 开发期调试入口，不再扩展。

### 分工

```
UE5 游戏实例：
  - 工具执行器（进程内，无转发）
  - MCP Server（对外暴露 tools/prompts，外部 Agent 直连）
  - 通过 REST 向 Bridge 注册 agent/tool 定义（供 ReAct 对话用）

Bridge (Go)：
  - ReAct 对话引擎（/api/chat SSE）
  - backend 注册中心（缓存 UE5 上报的定义，用于对话 + 恢复；通过 mcp.Registry 接口暴露）
```

### 待办

- [ ] UE5 C++ 实现 MCP Server（FHttpServer / WebSocket）
  - [ ] JSON-RPC 2.0 请求分发（initialize / ping / tools/list / tools/call / prompts/list / prompts/get）
  - [ ] SSE 长连接替代方案（WebSocket / 轮询 / 原始 socket）
  - [ ] agent/tool 定义本地加载（yaml/json）+ 向 Bridge 同步
- [ ] Go 侧 `internal/mcp/` 处置：保留作参考 or 删除（联调后决定）
- [ ] 协议规格文档（见 docs/UE5-MCP-SERVER.md 制作教程）

---

## P2 — NPC 管理

- [ ] 拆出轻量 `service/` 层（会话生命周期 + NPC 编排）
- [ ] `store/npc.go` — NPCStore 接口 + mem 实现
- [ ] NPC profile 可配置（JSON/YAML 加载）
- [ ] 多 NPC 并发会话
- [ ] 会话超时/自动清理

## P3 — 记忆系统（RAG 路线）

- [ ] 短期记忆上下文窗口（已完成：State.Messages）
- [ ] 长期记忆摘要 + 事实库召回 → 注入 prompt
- [ ] MemoryStore 接口

## P4 — 引擎对接（除 MCP 外）

- [ ] 协议定型（批量 SSE + error + 鉴权已定稿）
- [ ] UE5 C++ 插件（NPC 对话客户端：调 /api/chat）

## P5 — 异步蒸馏（独立远期路线，与 weight-masking 实验联动）

- [ ] 轻量模型集成
- [ ] 后台蒸馏任务
- [ ] LoRA weight 管理

## 📌 学习方向（不阻塞主线）

- 微服务化：store 接口隔离已预留，P0~P2 不做，后续拆 gRPC 客户端练手
