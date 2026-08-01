# UE5 端 MCP Server 实现教程

> 目标：在 UE5 游戏实例进程内实现 MCP Server，对外暴露该实例的 tools/prompts。
> 外部 Agent（MCP Client）直连 UE5，工具调用在游戏进程内完成，零转发延迟。
> 本教程基于 Go 侧 `internal/mcp/` 的协议实现（参考实现），给出 UE5 C++ 落地规格。

---

## 1. 背景与分工

```
外部 Agent (MCP Client) ──JSON-RPC──▶ UE5 游戏实例 (MCP Server, 进程内)
                                            │ 工具执行: 动画/通信/场景 (进程内)
                                            │
                                            │ agent/tool 定义注册 (REST → Bridge)
                                            ▼
                                    Bridge (Go) ReAct 对话引擎
```

- **UE5 实例**：MCP Server + 工具执行器（本教程内容）
- **Bridge**：ReAct 对话（`/api/chat`）+ UE5 注册中心（`/api/v1/ue5`）

**为什么放 UE5 端：**
1. 工具调用零转发——模型要播放动画，直接调 UE5 动画系统
2. 按实例天然隔离——每个游戏服一个 MCP Server
3. Bridge 不需要维护 MCP 长连接——专注对话

---

## 2. 协议速览（照抄 Go 参考实现）

### 2.1 传输

| 方式 | 路径 | 说明 |
|------|------|------|
| POST | `/mcp` | JSON-RPC 2.0 单发/批量 |
| GET  | `/mcp` | SSE 长连接（变更通知；**UE5 端可选实现**，见 §5） |

### 2.2 方法

| 方法 | 请求 params | 响应 result |
|------|-------------|-------------|
| `initialize` | `{protocolVersion, clientInfo, capabilities?}` | `{protocolVersion, capabilities, serverInfo, instructions?}` |
| `ping` | — | `{}` |
| `tools/list` | — | `{tools: [{name, description, inputSchema}]}` |
| `tools/call` | `{name, arguments}` | `{content: [{type:"text", text}]}` |
| `prompts/list` | — | `{prompts: [{name, description, arguments}]}` |
| `prompts/get` | `{name, arguments?}` | `{description, messages: [{role, content}]}` |

### 2.3 错误码（JSON-RPC 标准）

```
-32700 Parse error     -32600 Invalid Request
-32601 Method not found  -32602 Invalid params
-32603 Internal error
```

### 2.4 通知（SSE 推送）

```
notifications/tools/list_changed
notifications/agents/list_changed
```

---

## 3. 核心实现（C++ 骨架）

### 3.1 数据源：agent/tool 定义

UE5 侧维护两份本地注册表（JSON/YAML 加载，也可从 Bridge 同步）：

```cpp
// 运行时结构（示意）
struct FToolDef {
    FString Name;
    FString Description;
    TSharedPtr<FJsonObject> Parameters;  // JSON Schema object
};

struct FAgentDef {
    FString Name;          // 如 npc_alice
    FString Type;          // actor / system
    FString SystemPrompt;
    TArray<FString> Tools; // 引用 FToolDef::Name
};

class FRegistry {
    TMap<FString, FToolDef> Tools;
    TMap<FString, FAgentDef> Agents;
};
```

> ⚠️ 工具定义必须与注册到 Bridge 的版本**一致**（Bridge 侧 ReAct 用同一份定义让 LLM 生成调用）。

### 3.2 JSON-RPC 分发器

```cpp
// 输入: 请求 JSON 字符串 → 输出: 响应 JSON 字符串
FString DispatchMCP(const FString& RequestJson)
{
    // 1. 解析为 FJsonObject
    // 2. 取 method / id / params
    // 3. switch 分发:
    //    - "initialize"  → HandleInitialize()
    //    - "ping"        → RPC_OK({})
    //    - "tools/list"  → HandleToolsList()
    //    - "tools/call"  → HandleToolsCall(params)
    //    - "prompts/list"→ HandlePromptsList()
    //    - "prompts/get" → HandlePromptsGet(params)
    //    - 其他           → RPC_Error(id, -32601, "未知方法: " + method)
    // 4. 无 id 的通知: 不回复
}
```

**响应统一格式：**
```json
{"jsonrpc":"2.0","id":"1","result":{...}}
{"jsonrpc":"2.0","id":"1","error":{"code":-32602,"message":"..."}}
```

### 3.3 各方法实现要点

**tools/list** — 遍历 FRegistry::Tools，转协议格式：
```json
{
  "tools": [{
    "name": "play_animation",
    "description": "让 NPC 播放指定动画",
    "inputSchema": {"type":"object","properties":{"anim":{"type":"string","description":"动画名"}},"required":["anim"]}
  }]
}
```

**tools/call** — 核心！参数 → 执行 → 结果：
```cpp
FString HandleToolsCall(const TSharedPtr<FJsonObject>& Params)
{
    FString Name = Params->GetStringField("name");
    const TSharedPtr<FJsonObject>* ArgsPtr = nullptr;
    Params->TryGetObjectField("arguments", ArgsPtr);

    if (!Registry.Tools.Contains(Name))
        return RPC_Error(Id, -32602, TEXT("工具未注册: ") + Name);

    // 分发到游戏侧实际执行器
    FString ResultText = ExecuteGameTool(Name, ArgsPtr ? *ArgsPtr : nullptr);

    // 响应: content 数组
    return RPC_OK(Id, MakeJson(
        "content", MakeArray(MakeJson(
            "type", "text",
            "text", ResultText)));
}
```

**ExecuteGameTool** 是游戏侧接线点：
```cpp
FString ExecuteGameTool(const FString& Name, const FJsonObject& Args)
{
    if (Name == "play_animation") {
        FString Anim = Args.GetStringField("anim");
        // → 调用动画系统: AnimInstance->PlayAnimation(...)
        return TEXT("{\"played\":true,\"anim\":\"") + Anim + TEXT("\"}");
    }
    if (Name == "npc_say") {
        // → 调用对话系统，广播给周围玩家
        return TEXT("{\"said\":true}");
    }
    return TEXT("{\"error\":\"unknown tool\"}");
}
```

> 返回文本会作为 tool 结果回馈给 LLM，让模型继续推理。**结构化 JSON 比自由文本更适合模型消费。**

**prompts/list** — 遍历 Agents：
```json
{"prompts":[{"name":"npc_alice","description":"扮演 npc_alice(类型 actor)","arguments":[{"name":"player_message","description":"玩家对 NPC 说的一句话","required":true}]}]}
```

**prompts/get** — 组装 system + user 消息：
```json
{
  "description": "NPC npc_alice 的角色 prompt",
  "messages": [
    {"role":"system","content":{"type":"text","text":"你是面包店老板娘艾丽丝...\n- 你可用工具: play_animation"}},
    {"role":"user","content":{"type":"text","text":"{{player_message}} 或实际消息"}}
  ]
}
```

---

## 4. HTTP 接入（UE5 方案选型）

### 方案 A：FHttpServer（UE5 内置，推荐起步）

```cpp
#include "HttpServerModule.h"
#include "IHttpServer.h"

void StartMCPServer()
{
    auto& Module = FHttpServerModule::Get();
    TSharedPtr<IHttpServer> Server = Module.CreateServer(8080);  // MCP 端口

    Server->RegisterHandler(TEXT("/mcp"), EHttpServerRequestVerbs::VERB_POST,
        [](const FHttpServerRequest& Req, const FHttpServerResponse& Resp, bool bHead) {
            // 读 body → DispatchMCP → 回写 JSON
            FString Body = Req.Body.ToString();
            FString Response = DispatchMCP(Body);
            Resp->WriteBody(TArray<uint8>((uint8*)TCHAR_TO_UTF8(*Response), Response.Len()));
        });

    Server->Start();
}
```

**限制：**
- FHttpServer 是**请求-响应模型**，不支持 SSE 长连接
- 需要插件启用 `HttpServer` 模块（.uproject / .Build.cs 加 `"HttpServer"`）

### 方案 B：WebSocket（推荐用于通知推送）

外部 Agent 用 SSE 还是 WebSocket 取决于客户端。MCP 规范本身是 SSE，
但 UE5 侧长连接建议 WebSocket：
- 工具：`WebSockets` 插件（`FWebSocket` / 社区 `HTML5Networking`）
- 变更通知统一走 WS 推送，兼容性最好

### 方案 C：原始 socket（不推荐，除非自研传输）

自己起 `FTcpListener` 实现 HTTP/1.1 + chunked 或自定义协议。
工作量大，仅当 A/B 都不可用时考虑。

**推荐：起步用 A（POST JSON-RPC），通知推送用 B（WebSocket）。**
外部 Agent 侧两套传输都支持（OpenAI Agents SDK / MCP 官方 SDK 均可配）。

---

## 5. 变更通知（可选，第二阶段做）

MCP 规范里 SSE 推送 `list_changed` 通知，客户端据此重新拉取。

**UE5 端简化方案（三选一）：**

| 方案 | 实现 | 适用 |
|------|------|------|
| WebSocket 推送 | 连接池广播 JSON-RPC 通知 | 客户端支持 WS（推荐） |
| 轮询 | 客户端定时 `tools/list` 对比 | 客户端最简单，有延迟 |
| SSE | FHttpServer 不支持，需自建 | 不推荐 |

> 客户端断开/重连后，需要重新 `initialize`。UE5 端把连接 ID 和订阅的实例绑定即可。

---

## 6. 与 Bridge 的注册同步

**规则：UE5 是定义源头，Bridge 是副本。**

```
UE5 侧 (agent/tool 定义变更时)              Bridge (Go)
  POST /api/v1/ue5/instances/{id}/tools/{name}  ──▶ UpsertTool
  POST /api/v1/ue5/instances/{id}/agents/{name} ──▶ UpsertAgent
  (可带 X-UE5-Key header 鉴权)
```

- 启动时：UE5 全量上报一次（批量接口 `POST .../tools`、`POST .../agents`）
- 运行中：增删改即时上报（同名覆盖 = 热更新）
- Bridge 落盘，重启自动恢复——UE5 侧重启后重新上报即可

---

## 7. 验收清单

- [ ] `POST /mcp` 返回 initialize 握手成功
- [ ] `tools/list` 返回 UE5 注册的全部工具
- [ ] `tools/call play_animation {"anim":"wave"}` → UE5 实际播放动画 → 返回结果
- [ ] `prompts/get npc_alice` 返回 role prompt
- [ ] 未知方法返回 -32601，未知工具返回 -32602
- [ ] 工具定义变更后，Bridge 侧注册同步更新（curl 验证）
- [ ] （可选）WebSocket 通知推送 list_changed

---

## 8. 参考实现（Go）

协议逻辑的权威参考在 Bridge 仓库：
- `internal/mcp/protocol.go` — JSON-RPC 类型 + 错误码
- `internal/mcp/server.go` — 方法分发 + 各 handler
- `internal/mcp/registry.go` — Registry 通用接口（数据面/执行面抽象）
- `internal/backend/types.go` — agent/tool 定义结构（原 internal/ue5，2160ec9 更名）
- `internal/server/handler/mcp_handler.go` — HTTP/SSE 传输层

对照 §3 骨架逐方法抄即可。

> 2026-08-01 更新：`internal/ue5` 已更名 `internal/backend`；`internal/tool/` 已删除，
> 工具执行统一走 `mcp.Registry.ExecuteTool`（`backend.RegistryAdapter` 实现）。
