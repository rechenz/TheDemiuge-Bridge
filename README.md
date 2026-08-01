# TheDemiuge-Bridge

The gobackend of the DemiugeAgent — 游戏 NPC 接入 AI 的标准通信层（MCP Server + UE5 动态注册）。

## 架构概览

```
未来 Agent (MCP Client) ──JSON-RPC──▶  Bridge (MCP Server)  ──HTTP──▶  UE5 实例 (工具执行方)
                                            ▲
                                            │ REST 动态注册 agent/tool（热更新）
                                            │
                                         UE5 插件
```

- **Bridge = MCP Server**:对外暴露每个 UE5 实例的工具(动画控制等)与 NPC 角色 prompt
- **UE5 实例 = 注册生产方**:通过管理接口动态注册/更新/删除 agent 与 tool 定义,工具真正执行在 UE5 侧
- **按实例隔离**:每个 UE5 游戏服务器一套独立的 agent + tool 注册空间,互不干扰

## 快速开始

```bash
# 启动服务(本地联调默认不鉴权)
go run ./cmd/server

# 环境变量:
#   ADDR=:8080                  服务地址
#   UE5_REGISTRY_DIR=./registry 注册落盘目录(重启自动恢复)
#   UE5_API_KEY=xxx             UE5 管理接口鉴权(X-UE5-Key header,空则不鉴权)
#   UE5_DEFAULT_ENDPOINT=...    全局默认工具转发地址
#   DEEPSEEK_API_KEY=xxx        LLM 提供方 API Key(NPC 对话用)
#   CHAT_API_KEY=xxx            /api/chat 接口鉴权(X-API-Key header,空则不鉴权)
```

## 接口

### NPC 对话(`POST /api/chat`,SSE 流式)

Bridge 自己跑 ReAct 循环:玩家消息 → DeepSeek → 工具调用(转发 UE5) → 最终回复。

```bash
curl -N -X POST http://127.0.0.1:8080/api/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "instance_id": "inst_a",
    "agent": "npc_alice",
    "session_id": "player_1",
    "message": "早上好"
  }'
```

SSE 事件流(`data:` 行):

| 事件       | 字段                                  | 说明                             |
| ---------- | ------------------------------------- | -------------------------------- |
| `connected`| —                                     | 连接确认                         |
| `text`     | `{type, delta}`                       | 文本增量(流式)                   |
| `tool_call`| `{type, tool_call}`                   | 模型发起工具调用                 |
| `commentary`| `{type, commentary}`                 | 评述(推理/旁白,可忽略)           |
| `done`     | `{type, reply, usage?}`               | 对话结束,最终回复               |
| `error`    | `{type, error}`                       | 错误                             |

> 同一 `(instance_id, agent)` 对共享会话上下文;不同玩家用不同 `session_id` 隔离。

### UE5 管理接口(`/api/v1/ue5`,带 `X-UE5-Key` 鉴权)

| 方法        | 路径                            | 说明                               |
| ----------- | ------------------------------- | ---------------------------------- |
| POST        | `/instances`                    | 创建/注册实例                      |
| GET         | `/instances`                    | 列出全部实例                       |
| GET/DELETE  | `/instances/:id`                | 查询/注销实例                      |
| POST        | `/instances/:id/agents`         | 批量注册 agent(`agents.yaml` 清单) |
| POST/DELETE | `/instances/:id/agents/:name`   | 注册/删除单个 agent                |
| GET         | `/instances/:id/agents(/:name)` | 查询 agent 列表/单个               |
| POST        | `/instances/:id/tools`          | 批量注册 tool(`tools.yaml` 清单)   |
| POST/DELETE | `/instances/:id/tools/:name`    | 注册/删除单个 tool                 |
| GET         | `/instances/:id/tools(/:name)`  | 查询 tool 列表/单个                |

**注册一个工具(热更新,同名覆盖):**

```bash
curl -X POST http://127.0.0.1:8080/api/v1/ue5/instances/inst_a/tools/play_animation \
  -H 'Content-Type: application/json' \
  -d '{
    "description": "让 NPC 播放指定动画",
    "parameters": {
      "type": "object",
      "properties": {"anim": {"type": "string", "description": "动画名"}},
      "required": ["anim"]
    },
    "endpoint": "http://127.0.0.1:9000/mcp/call"
  }'
```

**注册一个 agent(引用已注册的工具):**

```bash
curl -X POST http://127.0.0.1:8080/api/v1/ue5/instances/inst_a/agents/merchant \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "actor",
    "system_prompt": "你是酒馆商人,热情但精明",
    "tools": ["play_animation"]
  }'
```

### MCP 入口(`/mcp/:instance_id`,未来 agent 调用)

```
POST /mcp/inst_a   JSON-RPC 2.0(单发/批量)
GET  /mcp/inst_a   SSE 长连接(工具/agent 变更通知)
```

支持的方法:

| 方法           | 说明                                                   |
| -------------- | ------------------------------------------------------ |
| `initialize`   | 协议握手(声明 tools/prompts 能力)                      |
| `ping`         | 存活探测                                               |
| `tools/list`   | 该实例注册的全部工具                                   |
| `tools/call`   | 转发到 UE5 执行并返回结果                              |
| `prompts/list` | 该实例全部 agent(映射为 prompt 模板)                   |
| `prompts/get`  | 按 agent 名获取角色 prompt(可传 `player_message` 参数) |

示例:

```json
{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"my-agent","version":"1.0"}}}
{"jsonrpc":"2.0","id":"2","method":"tools/list"}
{"jsonrpc":"2.0","id":"3","method":"tools/call","params":{"name":"play_animation","arguments":{"anim":"wave"}}}
{"jsonrpc":"2.0","id":"4","method":"prompts/get","params":{"name":"merchant","arguments":{"player_message":"你好"}}}
```

SSE 长连接会推送 `notifications/tools/list_changed` 与 `notifications/agents/list_changed`,客户端据此重新拉取。

## 注册数据模型(落盘)

```
registry/{instance_id}/
├── agents.yaml    # 总体 agent 注册
└── tools.yaml     # 总体 tool 注册
```

注册即落盘,重启自动恢复。