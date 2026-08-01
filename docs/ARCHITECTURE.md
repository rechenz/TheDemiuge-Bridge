# TheDemiuge-Bridge 架构设计

> 最后更新：2026-08-01（17:20）
> 状态：P0 核心对话跑通 ✅（ReAct 接线完成）；**MCP 协议层与后端彻底解耦 ✅（mcp.Registry 接口 + backend.RegistryAdapter）**；MCP Server 规划迁往 UE5 端（见 P1）

---

## 一、项目定位

游戏 NPC 接入 AI 的标准通信层。引擎无关，协议标准，开箱即用。

### 关键原则
- **引擎无关** — UE5 插件只是前端之一，协议层通用
- **通信标准化** — 定义游戏↔AI 的标准 API 接口
- **插件化 Agent** — NPC 能力通过 Tool 系统扩展
- **模块化** — 每个模块干一件事，不互相侵入

---

## 二、整体架构（最终形态）

```
┌─ UE5 游戏实例 ──────────────────────────────────────────────┐
│   NPC 实体管理 / Dialogue UI / 场景状态同步                  │
│   工具执行器（进程内,无转发）                                 │
│   MCP Server（对外暴露 tools/prompts,外部 Agent 直连）──2026-08-01 规划──▶│
│        │                                                     │
│        │ agent/tool 定义注册 (REST /api/v1/ue5)              │
└────────┼─────────────────────────────────────────────────────┘
         ▼
┌──────────────────────────────────────────────────────────────┐
│                  TheDemiuge Bridge (Go)                      │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │                   Server Layer (Hertz)               │    │
│  │  POST /api/chat        POST /api/v1/ue5/*             │    │
│  │  (NPC 对话 SSE)        (UE5 注册管理, X-UE5-Key)      │    │
│  │  /mcp/{instance_id}    /api/v1/health                 │    │
│  │  (MCP 入口,参考/调试用)                               │    │
│  └──────────────────────────┬───────────────────────────┘    │
│                             │                                │
│  ┌──────────────────────────┴───────────────────────────┐    │
│  │               Core Logic                             │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────┐  │    │
│  │  │ Agent        │  │ mcp.Registry │  │ LLM        │  │    │
│  │  │ (ReAct Runner│  │  (通用注册    │  │ (Provider  │  │    │
│  │  │  + 会话状态) │  │  接口,执行面) │  │  抽象层)   │  │    │
│  │  └──────┬───────┘  └──────┬───────┘  └─────┬──────┘  │    │
│  │         │                 │                 │         │    │
│  │         └─────────┬───────┴─────────────────┘         │    │
│  │                   ▼                                   │    │
│  │  ┌──────────────────────────────────────────────┐     │    │
│  │  │  backend.RegistryAdapter（实现 mcp.Registry）│     │    │
│  │  │  └─ Manager(落盘) + Client(HTTP 转发)             │     │    │
│  │  └──────────────────────────────────────────────┘     │    │
│  └────────────────────────────────────────────────────────┘    │
│                        │                                      │
│                        ▼                                      │
│              ┌──────────────────┐                             │
│              │  DeepSeek API    │                             │
│              │ (未来: OpenAI /   │                             │
│              │  本地模型)       │                             │
│              └──────────────────┘                             │
└──────────────────────────────────────────────────────────────┘

> 2026-08-01 架构决策：MCP Server 最终迁往 UE5 端（进程内,零转发）。
> Go 侧 internal/mcp/ 保留作协议参考与开发期调试入口,不再扩展。
> 玩家对话走 Bridge /api/chat；外部 Agent 通过 UE5 端 MCP 直连工具。
>
> **2026-08-01（17:00）架构重构（commit 62a0a2a + 2160ec9）**：
> - MCP 协议层与后端彻底解耦：新增 `mcp.Registry` 通用接口（数据面 Tools/Agents/GetXxx + 执行面 ExecuteTool），
>   `backend.RegistryAdapter` 把「UE5 注册中心 + HTTP 转发」实现为该接口。
> - `internal/ue5` 更名 `internal/backend`（包名 `ue5` → `backend`），注释通用化——backend 是通用后端接入层，UE5 只是其一实现。
> - 删除 `internal/tool/`（UE5Executor → chat.go `registryExecutor` + RegistryAdapter）、`internal/registry/`（YAML 静态注册）、
>   `config/agents.yaml` / `config/tools.yaml` / `RegistryConfig` / `ToolRegistry` / `AgentRegistry`。
> - 保留业务命名：`ue5_handler.go`、`/api/v1/ue5/*`、`X-UE5-Key`、`UE5Config`（对外接口不变）。
```

---

## 三、模块详解

### 3.1 Server Layer — `internal/server/` ✅（2026-08-01 完成）

> Hertz 初始化 + 路由注册在 `cmd/server/main.go`，handler 在 `internal/server/handler/`。
> 当前路由：

```
POST /api/chat            NPC 对话 SSE（X-API-Key 鉴权，CHAT_API_KEY）
/api/v1/ue5/*             UE5 注册管理（X-UE5-Key 鉴权）
/mcp/{instance_id}        MCP 入口（参考/调试用）
GET  /api/v1/health       健康检查
```

```
internal/server/handler/
├── chat.go               # POST /api/chat（SSE：text/tool_call/commentary/done/error）
├── ue5_handler.go        # UE5 注册管理 API
└── mcp_handler.go        # MCP 入口（JSON-RPC + SSE 变更通知）
```

**职责：**
- 路由分发到 handler
- 请求验证
- 响应格式化
- 不包含业务逻辑（业务在 agent/backend/mcp 层）

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
- **微服务化是学习方向（热尘个人目标）**：接口隔离正是微服务化的前置条件——未来拆成独立服务时，业务层依赖的是 interface，实现换成 gRPC 客户端即可。P0~P2 阶段不做微服务（YAGNI），但接口隔离从现在保留

**NPC 生命周期：**
```
Register → Loaded (场景加载) → Active (对话中)
                                              └→ Unloaded (离屏，异步蒸馏)
                                                              └→ Archived (彻底下线)
```

### 3.3 Agent Core — `internal/agent/` ✅（2026-08-01 完成）

> ReAct 循环已重建（`internal/agent/react.go`），基于新 types 体系。
> Prompt Builder 暂以 `agent.SystemPrompt + 工具列表` 形式在 Runner 内组装（buildMessages）。

AI 交互核心，Agent + ReAct。

```
internal/agent/
├── react.go              # Runner：ReAct 循环 + 历史窗口裁剪（maxHistoryMessages=30）+ GetAgent 热更新
├── react_test.go         # 单测（工具轮/单轮/最大轮次/无 executor）
└── react_history_test.go # 历史窗口裁剪单测（2026-08-01 275aa90）
```

**Agent.Run 流程：**
```
1. Prompt Builder 组装 system prompt
   └─ NPC 角色定义 + 记忆摘要 + 当前上下文 + 可用工具描述
2. 调用 LLM（流式/非流式）
3. 如果有 tool_calls → 执行工具 → 结果回馈 → 继续请求 LLM
4. 返回最终回复
```

**流式 + tool_call 的处理（重要细节）：**

DeepSeek 流式响应中 tool_calls 是增量出现的（delta.tool_calls 携带 index/id/name/arguments 片段）。因此 Agent 的流式对话流程是：

```
1. 流式收 token，同时累积拼接 tool_calls 增量
2. 流结束（收到 finish_reason）后统一判断：
   ├─ finish_reason == tool_calls → 执行工具 → 把结果回馈 → 重新请求 LLM
   └─ finish_reason == stop → 流式推送最终文本给客户端
```

**关键点：** 流式过程中不能边收边执行工具——必须等流结束拿到完整 tool_calls 才能执行。客户端看到的顺序是：先收到文本流（可能为空），若模型决定调用工具，最后收到工具调用事件，Agent 内部循环完成后才输出最终回复。

**Service 层决策（2026-07-31）：**

原架构图里有「Service」层但模块清单里没有，对不上。决策：
- **P0 阶段：不设 Service 层**，handler → agent 直连，少一层空转
- **P1 阶段（多 NPC 并发会话）**：拆出轻量 service 层（会话生命周期 + NPC 编排），handler 只做 HTTP 解析
- 因此 4.1 数据流图中的 Service 列在 P0 表示 handler 内的编排逻辑（虚线），P1 起独立成层

### 3.4 Tool System — `internal/tool/`（已删除）+ `mcp.Registry`（2026-08-01 重构）

> **commit 62a0a2a 重构**：`internal/tool/` 包删除。工具执行统一走 `mcp.Registry.ExecuteTool` 接口，
> 由后端适配器（`backend.RegistryAdapter`）实现——**ReAct 循环与任何特定后端解耦**。

**Registry 接口（`internal/mcp/registry.go`，本次重构核心）：**

```go
type Registry interface {
    // 数据面:从后端注册空间实时读取
    Tools(instanceID string) ([]types.Tool, bool)
    GetTool(instanceID, name string) (types.Tool, bool)
    Agents(instanceID string) ([]RegisteredAgent, bool)
    GetAgent(instanceID, name string) (RegisteredAgent, bool)
    // 执行面:按后端协议执行工具
    ExecuteTool(ctx context.Context, instanceID, name string, args map[string]any) (string, error)
}
```

**调用方（两个，都只认接口）：**
1. `mcp.Server` — tools/call、tools/list、prompts/list、prompts/get
2. `handler.ChatHandler` — ReAct 工具调用经 `registryExecutor`（实现 `agent.ToolExecutor`）委托 ExecuteTool

**执行流程（UE5 后端）：**
1. `registryExecutor.Execute` 解析模型参数 JSON（非法时回馈错误给模型）
2. `RegistryAdapter.ExecuteTool`：从 backend.Manager 取工具注册条目（按实例隔离）
3. `backend.Client.Forward` 转发到后端侧（地址解析：工具 Endpoint → 实例 DefaultEndpoint → 全局默认）
4. 返回结果文本（结构化 result 序列化 / 自由文本原样）

**未来本地工具（2026-07-31 设计保留，暂未实现）：**

手写 JSON Schema 繁琐易错。预留反射工具 `FromStruct(v any) (*JSONSchema, error)`，让工具作者用 struct + tag 定义参数，自动生成 schema。届时实现一个直接调用的 Registry 实现即可，协议层零改动。

### 3.5 LLM Client — `internal/llm/` ✅（已完成 2026-08-01）

2026-08-01 基于新的 `internal/types` 完整类型体系重建完成，
补齐 Tools 通道、真实 finish_reason、BaseURL 可配置、http.Client 可注入、
错误分类与 httptest 单测。

LLM 提供者抽象层，统一入口。

```
internal/llm/
├── provider.go           # Provider 接口（上层唯一依赖）
├── deepseek_provider.go  # DeepSeek Provider 实现（NewDeepseekProvider）
└── deepseek/             # DeepSeek 客户端实现
    ├── deepseek.go       # Client 结构体 + 错误分类哨兵 + 流式 SSE 解析
    ├── chat.go           # 非流式入口 Chat
    ├── chat_stream.go    # 流式入口 ChatStream
    └── *_test.go         # httptest 单测（流式解析 / tool_call 拼接 / 错误分类）
```

**关键设计：两个入口，不强行统一（2026-07-31 修订，2026-08-01 落地）**

~~旧设计：无论流式/非流式都返回 `<-chan Token`~~ —— channel 有泄漏风险、错误传递别扭、tool call 混在流里判断麻烦。改为：

```go
// 非流式：直接返回完整响应
func Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

// 流式：回调推 token，错误通过返回值传递
func ChatStream(ctx context.Context, req ChatRequest, onToken func(Token) error) error

type ChatRequest struct {
    Messages    []types.Message
    Tools       []types.Tool
    MaxTokens   int
    Temperature float32
    // ... 其余参数由 config 提供
}

type Token struct {
    Content          string  // 文本增量
    ReasoningContent string  // 推理增量
    // 注意：tool_calls 增量由客户端内部拼接，
    // 流结束统一通过 ChatResponse.ToolCalls 返回，不混在 Token 里
}
```

**设计要点：**
- 非流式：一次调用，直接返回 `ChatResponse`（含 message + tool_calls + usage）
- 流式：回调收增量，**tool_calls 由客户端内部拼接**，流结束时随响应返回——Agent 层不用关心 DeepSeek 的 delta 协议细节
- **finish_reason 保留真实值**：流式循环按 choice index 收集最后出现的 finish_reason（tool_calls / length / content_filter 等），不再硬编码 stop——Agent 层据此判断"是否执行工具 / 是否截断"
- **base URL 可配置**：`DEEPSEEK_BASE_URL` 环境变量 / `config.BaseURL`，默认 DeepSeek 官方地址；换 OpenAI / 本地兼容服务零改动
- **http.Client 可注入**：`config.HTTPClient`（单测注入 httptest 客户端）+ `config.HTTPTimeout`（默认 60s）
- **错误分类可编程**：API 错误解析为 `*deepseek.ClassifiedError`，Unwrap 出分类哨兵 `ErrAuth`（401/403）/ `ErrRateLimit`（429）/ `ErrServer`（5xx）/ `ErrTimeout`，上层用 `errors.Is` 判断重试/降级
- 单测：httptest mock 服务端，覆盖流式解析、tool_call 增量拼接、finish_reason 聚合、错误分类

### 3.6 Memory System — `internal/memory/`（TODO）

长期记忆管理。**2026-07-31 修订：记忆拆为三条独立路线，不要混着做：**

| 路线     | 技术方案                                                  | 阶段      | 状态         |
| -------- | --------------------------------------------------------- | --------- | ------------ |
| 对话记忆 | 上下文窗口（LLM 原生，`State.Messages`）                  | P0 即生效 | 无需额外开发 |
| 长期记忆 | 摘要 + 事实库召回 → 注入 prompt（RAG 路线）               | P2        | TODO         |
| 蒸馏记忆 | 离线微调 → LoRA weight dump（与 weight-masking 实验联动） | P4        | 远期         |

**接口（P2 落地）：**
```go
// 短期：Agent 上下文（已在 State.Messages，无需接口）
// 长期：
Store(npcID, context)   // 摘要/事实写入
Recall(npcID) → Memory  // 召回 → 注入 prompt
```

> ⚠️ 注意：P2 的「记忆召回 → 注入 prompt」是 RAG/摘要路线，与 P4 的 LoRA 蒸馏是**两套完全不同的技术**，互不替代。P4 蒸馏是独立远期实验（对齐 weight-masking 实验结论：Fixed Mask 在困难数据集反超 baseline），不阻塞 P0~P3。

### 3.7 Config — `internal/config/` ✅（已完成）

环境变量加载 + `ToChatRequest()` 映射到 DeepSeek 请求体，覆盖 Thinking / ReasoningEffort / ResponseFormat / Stop / StreamOptions 等可选字段。
- 2026-08-01（62a0a2a）：删除 `RegistryConfig`（AGENTS_FILE / TOOLS_FILE 环境变量），静态 YAML 注册废弃

### 3.8 Types — `internal/types/` ✅（已完成，重构地基）

覆盖 DeepSeek Chat Completions 官方文档全部类型：

```
internal/types/
├── message.go    # Message 接口 + System/User/Assistant/Tool 四类消息（含显式 Role + NewXxx 构造器）
├── deepseek.go   # 请求体 / 非流式响应 / SSE chunk / Usage / Logprobs / 错误体
├── tools.go      # Tool 定义 + JSON Schema + ToolCall + ToolChoice
└── agent.go      # Agent / AgentState / SessionContext（纯数据类型）
```

**设计要点：**
- 所有消息类型实现统一 `Message` 接口（`isMessage()` 保证包内封闭）
- `StopSequence` / `ToolChoice` 自定义 Marshal/Unmarshal，兼容官方 oneOf 语义
- `DeepSeekAPIError` 实现 error 接口，便于 agent 层判断重试/降级
- 后续换 OpenAI / 本地模型只需新增 types + provider，不动上层
- 2026-08-01：消息加显式 `Role` 字段（275aa90）；删除 `ToolRegistry` / `AgentRegistry`（62a0a2a，注册职责交给 mcp.Registry）

---

## 四、数据流

### 4.1 NPC 对话（核心流程）

```
客户端                     Server              Agent               LLM
  │                         │                    │                  │
  │ POST /api/chat          │                    │                  │
  │ {npc_id, message}       │                    │                  │
  │────────────────────────→│                    │                  │
  │                         │  Parse request     │                  │
  │                         │  (P0: handler 内   │                  │
  │                         │   编排；P1: 拆 service)              │
  │                         │ Get/Session        │                  │
  │                         │───────────────────→│                  │
  │                         │ Get/NPC identity   │                  │
  │                         │───────────────────→│                  │
  │                         │                    │ Append user msg  │
  │                         │                    │ → state          │
  │                         │                    │ Build system     │
  │                         │                    │ prompt           │
  │                         │                    │──────────────────→│
  │                         │                    │  Chat request    │
  │                         │                    │──────────────────→│
  │                         │                    │   ← stream tokens│
  │                         │ ← stream tokens   │                  │
  │  ← stream tokens       │                    │                  │
  │                         │                    │ finish_reason ==  │
  │                         │                    │ tool_calls?       │
  │                         │                    │ → Exec tool       │
  │                         │                    │ → Loop LLM        │
  │                         │                    │  ← final response │
  │                         │ ← final response  │                  │
  │  ← final response      │                    │                  │
```

> P0 阶段 handler 内完成会话/NPC 编排（相当于数据流里的 Service 职责）；P1 起独立为 `service/` 层。
>
> 工具执行环节（finish_reason == tool_calls）：Runner → handler.registryExecutor → mcp.Registry.ExecuteTool
> → backend.RegistryAdapter → backend.Client.Forward → UE5 游戏实例进程内执行 → 结果文本回馈 LLM。
> 玩家侧通过 SSE 实时收到 `text` / `tool_call` / `commentary` / `done` / `error` 事件（见 chat.go 协议）。

### 4.2 NPC 离屏记忆蒸馏（远期，独立路线）

> 与 P2 的 RAG 记忆互不替代，是独立实验路线。

```
场景卸载 → 触发异步蒸馏任务
            → Agent 用 NPC 记忆 + 对话历史 微调轻量模型
            → 保存 LoRA weight (百 KB 级)
            → 下次场景加载时 merge 回
```

---

## 五、当前状态对比（2026-08-01 更新）

| 模块      | 应有                       | 现有                                                        | 状态                                       |
| --------- | -------------------------- | ----------------------------------------------------------- | ------------------------------------------ |
| `types/`  | 完整 API 类型体系          | `message.go` + `deepseek.go` + `tools.go` + `agent.go`      | ✅ 已完成                                   |
| `config/` | 配置加载 + 请求映射        | `config.go`（ToChatRequest + CHAT_API_KEY）                 | ✅ 已完成                                   |
| `server/` | Hertz + handler            | main.go 接线 + handler/{chat,ue5_handler,mcp_handler}       | ✅ 已完成（2026-08-01）                     |
| `agent/`  | Agent Run + ReAct          | `react.go`（Runner + EventSink + 评述归档 + 历史窗口裁剪）  | ✅ 已完成（2026-08-01）                     |
| `tool/`   | ToolExecutor + 工具实现    | ~~`ue5_executor.go`~~ **已删除**，执行走 `mcp.Registry.ExecuteTool` | ✅ 已重构（2026-08-01 62a0a2a）             |
| `llm/`    | 统一入口                   | provider + deepseek/（含单测）                              | ✅ 已重建（2026-08-01）                     |
| `mcp/`    | MCP 协议层                 | 协议分发 + SSE Hub + **Registry 接口** + tools/prompts     | ✅ 已完成；**规划迁往 UE5 端**（P1）         |
| `backend/`| 通用后端接入层（原 ue5/）  | RegistryAdapter + Manager + Instance + Client（落盘/转发/变更广播） | ✅ 已完成（2026-08-01，2160ec9 更名）       |
| `registry/`| YAML 静态注册             | ~~decode/load/load_test~~ **已删除**                       | 🗑 已删除（62a0a2a）                         |
| `store/`  | NPC/Session/Memory 接口    | 无                                                          | ❌ 未开工（P2，接口隔离为微服务学习方向预留） |

**当前可编译可运行**（`go build` / `go vet` / `go test` 全绿）：
- 玩家对话：`POST /api/chat` → ReAct → DeepSeek → mcp.Registry.ExecuteTool（backend 转发）→ SSE 回推
- 注册管理：`/api/v1/ue5/*` 动态注册实例/agent/tool，落盘恢复
- MCP：`/mcp/{instance_id}` JSON-RPC（保留作参考），通过 Registry 接口读取/执行

**2026-07-31 架构评审结论：**
- LLM 统一入口：`<-chan Token` → **回调式两入口**（Chat / ChatStream）
- Tool 参数：手写 schema → **FromStruct 反射**
- Service 层：P0 不设，handler 直连 agent；P1 多 NPC 并发时再拆
- Memory：拆为对话记忆（P0）/ 长期 RAG（P2）/ 蒸馏（P4）三条独立路线
- SSE：每 token 一事件 → **批量推送 + error 事件 + X-API-Key 鉴权**
- 微服务化：保留为学习方向，接口隔离前置

---

## 六、迭代路线图（P0→P4）

### P0 — 核心对话跑通（当前）

**已完成：**
- [x] `internal/types/` 完整重写（message / deepseek / tools）
- [x] `internal/config/` 重写（环境变量 → DeepSeek 请求映射）
- [x] 提交 types/config 地基版本（待做）

**待完成（全部完成，2026-08-01）：**
1. [x] LLM 客户端重写（`internal/llm/`）：**回调式两入口** + Tools 通道 + finish_reason 保留 + 错误分类 + 单测
2. [x] Agent 层重建（`internal/agent/`）：Run 循环 + Prompt Builder + tool_calls 循环（流结束再判断工具调用）+ 历史窗口裁剪
3. [x] Tool 系统：~~`internal/tool/`~~ → **`mcp.Registry` 接口 + `backend.RegistryAdapter`**（62a0a2a；FromStruct 反射留待本地工具路线）
4. [x] Server 层重建（`internal/server/`）：chat handler + 路由注册 + main.go 接线（P0 不设 service 层）
5. [x] 协议落地：**SSE 事件 + error 事件 + X-API-Key 鉴权**
6. [x] 验证：集成测试（真实 Hertz + mock UE5 + mock LLM 全链路）替代本地 curl 验证
7. [x] 提交 P0 完成版本（34cbb4e ~ 2160ec9）

### P1 — NPC 管理
1. 拆出轻量 `service/` 层（会话生命周期 + NPC 编排，P0 时在 handler 内）
2. `service/npc.go` — NPC 注册/查询
3. NPC 角色 prompt 可配置（不再是硬编码）
4. 多 NPC 并发会话
5. 会话超时/自动清理

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

**鉴权（2026-07-31 新增）：** 所有 `/api/v1/*` 路由要求 `X-API-Key` header，配置项 `API_KEY`（空则仅允许 localhost，P0 阶段默认本地联调）。

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

**响应格式（SSE，2026-07-31 优化为批量推送）：**

> ❌ 旧设计：每 token 一个事件、每个都是独立 JSON —— 游戏引擎侧解析开销大。
> ✅ 新设计：按批推送（攒 ~20-50 字或按句号/停顿切分），减少事件数；文本可整体放入一个事件。

```
data: {"type":"text","content":"欢迎光临！请问需要点什么？"}
data: {"type":"tool_call","name":"wave_hand","args":{"target":"player"}}
data: {"type":"done"}
```

**事件类型：**

| type        | 含义                                                | 字段                                 |
| ----------- | --------------------------------------------------- | ------------------------------------ |
| `text`      | 文本增量（批）                                      | `content`                            |
| `reasoning` | 推理内容增量（可选，思考模式）                      | `content`                            |
| `tool_call` | 工具调用（客户端内部拼接完成后发出）                | `name`, `args`                       |
| `error`     | 错误事件（新增）                                    | `code`, `message`                    |
| `done`      | 流结束                                              | —                                    |
| `usage`     | token 用量（可选，需 stream_options.include_usage） | `prompt_tokens`, `completion_tokens` |

**错误示例：**
```
data: {"type":"error","code":"rate_limit","message":"请求过于频繁"}
```

---

## 八、记录方式

从这次开始，项目状态持续记录在：
- `docs/ARCHITECTURE.md` — 架构设计（本文档，随仓库版本管理）
- `docs/TODO.md` — 任务追踪（随仓库版本管理）
- `~/projects/TheDemiugeAgent/` 下的同名文件为旧副本，仅作备份
- `memory/` 日常日志记录进度
- `MEMORY.md` 提炼关键节点
