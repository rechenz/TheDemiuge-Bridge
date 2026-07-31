# TheDemiuge-Bridge 架构设计

> 最后更新：2026-07-31
> 状态：重构中（2026-07-30 完全重写，types/config 层已完成，llm/agent/server 待重建）

---

## 一、项目定位

游戏 NPC 接入 AI 的标准通信层。引擎无关，协议标准，开箱即用。

### 关键原则
- **引擎无关** — UE5 插件只是前端之一，协议层通用
- **通信标准化** — 定义游戏↔AI 的标准 API 接口
- **插件化 Agent** — NPC 能力通过 Tool 系统扩展
- **模块化** — 每个模块干一件事，不互相侵入

---

## 二、整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                    游戏引擎层 (任意引擎)                       │
│  ┌──────────────┐   ┌──────────────┐   ┌────────────────┐   │
│  │ NPC 实体管理  │   │ Dialogue UI  │   │ 场景状态同步    │   │
│  └──────┬───────┘   └──────┬───────┘   └───────┬────────┘   │
└─────────┼──────────────────┼───────────────────┼────────────┘
          │                  │                   │
          └──────────────────┼───────────────────┘
                             │   通信协议
                    (HTTP/SSE → WebSocket → gRPC)
                             ▼
┌──────────────────────────────────────────────────────────────┐
│                  TheDemiuge Bridge (Go)                      │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │                   Server Layer                       │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │    │
│  │  │ Router   │→ │ Middle   │→ │ Handler          │   │    │
│  │  │ (Hertz)  │  │(Auth/Log)│  │ (业务入口)        │   │    │
│  │  └──────────┘  └──────────┘  └────────┬─────────┘   │    │
│  └────────────────────────────────────────┼─────────────┘    │
│                                           │                  │
│  ┌────────────────────────────────────────┼─────────────┐    │
│  │               Core Logic               │             │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌┴──────────┐  │    │
│  │  │ Agent        │  │ Tool         │  │ LLM       │  │    │
│  │  │ (Prompt Build│  │ (Registry +  │  │ (Provider │  │    │
│  │  │  + ReAct)    │  │  Tool Impl)  │  │  抽象层)  │  │    │
│  │  └──────┬───────┘  └──────┬───────┘  └─────┬─────┘  │    │
│  └─────────┼─────────────────┼─────────────────┼────────┘    │
│            │                 │                 │             │
│            ▼                 ▼                 ▼             │
│  ┌──────────────────────────────────────────────────────┐    │
│  │                   Store 层（接口隔离）                 │    │
│  │                                                      │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────┐   │    │
│  │  │ NPCStore     │  │ SessionStore │  │ Memory   │   │    │
│  │  │ (接口)       │  │ (接口)       │  │ Store    │   │    │
│  │  │              │  │              │  │ (接口)   │   │    │
│  │  ├──────────────┤  ├──────────────┤  ├──────────┤   │    │
│  │  │ mem/         │  │ mem/         │  │ mem/     │   │    │
│  │  │ sqlite/      │  │ sqlite/      │  │ (TODO)   │   │    │
│  │  │ ...          │  │ ...          │  │          │   │    │
│  │  └──────────────┘  └──────────────┘  └──────────┘   │    │
│  └──────────────────────────────────────────────────────┘    │
│                        │                                      │
│                        ▼                                      │
│              ┌──────────────────┐                             │
│              │  DeepSeek API    │                             │
│              │ (未来: OpenAI /   │                             │
│              │  本地模型)       │                             │
│              └──────────────────┘                             │
└──────────────────────────────────────────────────────────────┘
```

---

## 三、模块详解

### 3.1 Server Layer — `internal/server/`（待重建）

> ⚠️ 2026-07-30 重写时已删除，当前 `cmd/server/main.go` 只是空壳（无任何路由）。
> 目标目录结构：

```
internal/server/
├── server.go       # Hertz 初始化 + 路由注册
├── middleware/     # 中间件（日志、鉴权、CORS）
└── handler/       # 请求处理器
    ├── chat.go    # NPC 对话
    ├── npc.go     # NPC CRUD
    ├── session.go # 会话管理
    └── health.go  # 健康检查
```

**职责：**
- 路由分发到 handler
- 请求验证
- 响应格式化
- 不包含业务逻辑

### 3.2 Store Layer — `internal/store/`（接口隔离）

**核心原则：调用方依赖接口，不依赖实现。

```
internal/store/
├── npc.go            # NPCStore 接口
├── session.go        # SessionStore 接口
├── memory.go         # MemoryStore 接口（TODO）
├── mem/              # 内存实现（当前默认）
│   ├── npc.go
│   └── session.go
└── sqlite/           # SQLite 实现（TODO）
    ├── npc.go
    └── session.go
```

**设计思想：**
- 所有 store 定义 interface，不暴露具体实现
- 当前是内存实现（快速开发）
- 以后加 SQLite 实现，业务代码一行不改
- 未来微服务化：接口不变，实现变 gRPC 客户端

**NPC 生命周期：**
```
Register → Loaded (场景加载) → Active (对话中)
                                              └→ Unloaded (离屏，异步蒸馏)
                                                              └→ Archived (彻底下线)
```

### 3.3 Agent Core — `internal/agent/`（待重建）

> ⚠️ 2026-07-30 重写时已删除（agent.go / tool.go / example_tool.go / types.go），需按新 types 体系重建。

AI 交互核心，Agent + Tool + ReAct。

```
internal/agent/
├── agent.go       # Agent 核心（Run 循环）
├── builder.go     # Prompt Builder
└── types.go       # 类型定义
```

**Agent.Run 流程：**
```
1. Prompt Builder 组装 system prompt
   └─ NPC 角色定义 + 记忆摘要 + 当前上下文 + 可用工具描述
2. 调用 LLM（流式/非流式）
3. 如果有 tool_calls → 执行工具 → 结果回馈 → 继续请求 LLM
4. 返回最终回复
```

### 3.4 Tool System — `internal/tool/`

NPC 能力扩展系统。

```
internal/tool/
├── registry.go    # Tool 注册中心
└── tools/         # 具体工具实现
    ├── time.go    # 获取时间
    ├── search.go  # 知识库搜索（TODO）
    └── ...
```

**Tool 接口：**
```go
type Tool interface {
    Name() string
    Description() string
    Parameters() JSONSchema
    Execute(ctx, args) → (result, error)
}
```

### 3.5 LLM Client — `internal/llm/`（待重写）

> ⚠️ 2026-07-31 工作区已删除旧实现（deepseek.go / completion.go），
> 待基于新的 `internal/types` 完整类型体系重建。

LLM 提供者抽象层，统一入口。

```
internal/llm/
├── client.go     # 统一入口（stream/non-stream）
├── deepseek.go   # DeepSeek 实现（用 types.DeepseekChatRequest/Response）
└── provider.go   # Provider 接口
```

**关键设计：只暴露一个入口**

```go
// client.go — 统一入口
func Chat(ctx context.Context, req ChatRequest) (<-chan Token, error)

type ChatRequest struct {
    Messages    []ChatMessage
    Tools       []ToolDef
    Stream      bool
    MaxTokens   int
    Temperature float32
}

type Token struct {
    Content string    // SSE content
    ToolID  string    // tool call id
    Done    bool
    Error   error
}
```

无论是流式还是非流式，都返回 `<-chan Token`。调用方通过 channel 读内容：
- 非流式：channel 只返回一个 Token（或分块）
- 流式：逐 token 推送
- tool call：标记为特殊 Token 类型

### 3.6 Memory System — `internal/memory/`（TODO）

长期记忆管理。

- **短期记忆：** Agent 上下文（已在 `State.Messages`）
- **长期记忆：** 离线蒸馏 → LoRA weight dump
- **接口：** `Store(npcID, context)` / `Recall(npcID) → Memory`

### 3.7 Config — `internal/config/` ✅（已完成）

2026-07-31 重写完成（工作区未提交）：环境变量加载 + `ToDeepseekRequest()` 映射到 DeepSeek 请求体，覆盖 Thinking / ReasoningEffort / ResponseFormat / Stop / StreamOptions 等可选字段。

### 3.8 Types — `internal/types/` ✅（已完成，重构地基）

2026-07-31 完整重写（工作区未提交），覆盖 DeepSeek Chat Completions 官方文档全部类型：

```
internal/types/
├── message.go    # Message 接口 + System/User/Assistant/Tool 四类消息
├── deepseek.go   # 请求体 / 非流式响应 / SSE chunk / Usage / Logprobs / 错误体
└── tools.go      # Tool 定义 + JSON Schema + ToolCall + ToolChoice
```

**设计要点：**
- 所有消息类型实现统一 `Message` 接口（`isMessage()` 保证包内封闭）
- `StopSequence` / `ToolChoice` 自定义 Marshal/Unmarshal，兼容官方 oneOf 语义
- `DeepSeekAPIError` 实现 error 接口，便于 agent 层判断重试/降级
- 后续换 OpenAI / 本地模型只需新增 types + provider，不动上层

---

## 四、数据流

### 4.1 NPC 对话（核心流程）

```
客户端                     Server              Service            Agent               LLM
  │                         │                    │                 │                  │
  │ POST /api/chat          │                    │                 │                  │
  │ {npc_id, message}       │                    │                 │                  │
  │────────────────────────→│                    │                 │                  │
  │                         │  Parse request     │                 │                  │
  │                         │────────────────────│                 │                  │
  │                         │ Get/Session        │                 │                  │
  │                         │───────────────────→│                 │                  │
  │                         │ Get/NPC identity   │                 │                  │
  │                         │───────────────────→│                 │                  │
  │                         │                    │ Append user msg │                  │
  │                         │                    │ → state         │                  │
  │                         │                    │────────────────→│                  │
  │                         │                    │                 │ Build system      │
  │                         │                    │                 │ prompt            │
  │                         │                    │                 │──────────────────→│
  │                         │                    │                 │  Chat request     │
  │                         │                    │                 │──────────────────→│
  │                         │                    │                 │   ← stream tokens │
  │                         │ ← stream tokens   │                 │                  │
  │  ← stream tokens       │                    │                 │                  │
  │                         │                    │                 │ If tool_call      │
  │                         │                    │                 │ → Exec tool       │
  │                         │                    │                 │ → Loop LLM        │
  │                         │                    │                  ← final response  │
  │                         │ ← final response  │                 │                  │
  │  ← final response      │                    │                 │                  │
```

### 4.2 NPC 离屏记忆蒸馏（远期）

```
场景卸载 → 触发异步蒸馏任务
            → Agent 用 NPC 记忆 + 对话历史 微调轻量模型
            → 保存 LoRA weight (百 KB 级)
            → 下次场景加载时 merge 回
```

---

## 五、当前状态对比（2026-07-31 更新）

| 模块 | 应有 | 现有 | 状态 |
|------|------|------|------|
| `types/` | 完整 API 类型体系 | `message.go` + `deepseek.go` + `tools.go`（737 行） | ✅ 已重写，未提交 |
| `config/` | 配置加载 + 请求映射 | `config.go`（ToDeepseekRequest） | ✅ 已重写，未提交 |
| `server/` | Hertz + handler | `cmd/server/main.go` 空壳 | ❌ 待重建 |
| `agent/` | Agent Run + Prompt Builder | 无 | ❌ 待重建（已删） |
| `tool/` | 注册 + 工具实现 | 无 | ❌ 待重建（已删） |
| `llm/` | 统一入口 | 无（旧实现已删） | ❌ 待重写 |
| `store/` | NPC/Session/Memory 接口 | 无 | ❌ 未开工（P1） |

**当前可编译**（`go build ./...` 通过），但运行是空服务器。

---

## 六、迭代路线图（P0→P4）

### P0 — 核心对话跑通（当前）

**已完成：**
- [x] `internal/types/` 完整重写（message / deepseek / tools）
- [x] `internal/config/` 重写（环境变量 → DeepSeek 请求映射）
- [x] 提交 types/config 地基版本（待做）

**待完成：**
1. [ ] LLM 客户端重写（`internal/llm/`）：ChatCompletion（非流式）+ ChatStream（SSE），基于新 types
2. [ ] Agent 层重建（`internal/agent/`）：Run 循环 + Prompt Builder + tool_calls 循环
3. [ ] Tool 系统（`internal/tool/`）：Registry + 示例工具（time.go）
4. [ ] Server 层重建（`internal/server/`）：chat handler + 路由注册 + main.go 接线
5. [ ] 验证：本地 curl → HTTP SSE → DeepSeek → tool call → 返回
6. [ ] 提交 P0 完成版本

### P1 — NPC 管理
1. `service/npc.go` — NPC 注册/查询
2. NPC 角色 prompt 可配置（不再是硬编码）
3. 多 NPC 并发会话
4. 会话超时/自动清理

### P2 — 记忆系统
1. 短期记忆上下文窗口管理
2. 长期记忆持久化（文件/SQLite）
3. 记忆召回 → 注入 prompt

### P3 — 引擎对接
1. 通信协议定型（HTTP/SSE 或 WebSocket）
2. UE5 C++ 插件
3. 协议文档 → 开源

### P4 — 异步蒸馏
1. 轻量模型集成
2. 后台蒸馏任务调度
3. LoRA weight 保存/加载

---

## 七、协议设计（草案）

当前使用 HTTP + SSE，未来可以升级。

```
POST /api/v1/chat                      — 对话（流式/非流式）
POST /api/v1/npc                       — 注册 NPC
GET  /api/v1/npc/:id                   — 查询 NPC
DELETE /api/v1/npc/:id                 — NPC 下线
POST /api/v1/memory/:npc_id/distill    — 触发记忆蒸馏
GET  /api/v1/health                    — 健康检查
```

**请求格式：**
```json
{
    "npc_id": "merchant_001",
    "session_id": "abc123",
    "message": "你这里卖什么？",
    "stream": true,
    "context": {
        "scene": "market_square",
        "time": "day",
        "player_level": 5
    }
}
```

**响应格式（SSE）：**
```
data: {"type":"token","content":"欢"}
data: {"type":"token","content":"迎"}
data: {"type":"token","content":"光"}
data: {"type":"token","content":"临"}
data: {"type":"token","content":"！"}
data: {"type":"tool_call","name":"wave_hand"}
data: {"type":"done"}
```

---

## 八、记录方式

从这次开始，项目状态持续记录在：
- `docs/ARCHITECTURE.md` — 架构设计（本文档，随仓库版本管理）
- `docs/TODO.md` — 任务追踪（随仓库版本管理）
- `~/projects/TheDemiugeAgent/` 下的同名文件为旧副本，仅作备份
- `memory/` 日常日志记录进度
- `MEMORY.md` 提炼关键节点
