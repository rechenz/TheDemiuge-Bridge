# TheDemiuge-Bridge 测试指令集

> 基于 2026-08-04 实测验证的全套测试指令（build/vet/test 全绿 + HTTP 冒烟全通）。
> 项目位置（WSL）：`/home/rechenz/projects/TheDemiugeAgent/TheDemiuge-Bridge`
> 所有命令在 WSL 的 bash 里执行。

---

## 0. 环境速查

| 环境变量               | 默认值              | 说明                                       |
| ---------------------- | ------------------- | ------------------------------------------ |
| `ADDR`                 | `:8080`             | 监听地址                                   |
| `DEEPSEEK_API_KEY`     | 空                  | LLM key，**对话接口必需**                  |
| `CHAT_API_KEY`         | 空                  | `/api/chat` 鉴权（X-API-Key），空=不鉴权   |
| `UE5_API_KEY`          | 空                  | 管理接口鉴权（X-UE5-Key），空=不鉴权       |
| `UE5_REGISTRY_DIR`     | `./registry`        | 注册信息落盘目录                           |
| `UE5_DEFAULT_ENDPOINT` | 空                  | 工具执行默认转发地址                       |
| `MODEL_NAME`           | `deepseek-v4-flash` | 模型名                                     |
| `HTTP_CLIENT`          | 空                  | `insecure` = 跳过 TLS 校验（本地 mock 用） |

---

## 1. 静态检查 + 自动化测试

```bash
cd /home/rechenz/projects/TheDemiugeAgent/TheDemiuge-Bridge

# 基础三连（应全绿）
go build ./... && go vet ./... && go test ./...

# 全量单测（含 mock，不需要 API key）
go test -v ./...

# 热更新专项（工具热加载 / prompt 热加载，对应「实时改 tool」场景）
go test -v ./internal/server/handler/ -run 'TestChatHandler_ToolHotReload|TestChatHandler_PromptHotReload'

# 单包测试
go test -v ./internal/agent/        # ReAct 循环
go test -v ./internal/backend/      # 注册中心 Manager
go test -v ./internal/mcp/          # MCP 协议 + Hub 广播
go test -v ./internal/llm/deepseek/ # LLM 客户端（mock）
```

---

## 2. 启动服务

```bash
cd /home/rechenz/projects/TheDemiugeAgent/TheDemiuge-Bridge

# ⚠️ 必须 nohup + disown：WSL 会话一关，服务收 SIGHUP 会优雅退出
# 本地联调（无 LLM key，可测管理接口 + MCP + 热更新）
UE5_API_KEY=testkey UE5_REGISTRY_DIR=/tmp/reg \
  nohup go run ./cmd/server > /tmp/bridge.log 2>&1 & disown

# 完整模式（真对话）
DEEPSEEK_API_KEY=sk-xxxx CHAT_API_KEY=chatkey UE5_API_KEY=testkey \
  nohup go run ./cmd/server > /tmp/bridge.log 2>&1 & disown

# 健康检查
curl -s http://127.0.0.1:8080/api/v1/health
# → {"instances":0,"status":"ok"}
```

**停服务**（进程是 `go run` 父进程 + `.cache/go-build/.../server` 子进程，pkill 容易踩坑）：

```bash
pkill -f 'go run ./cmd/server'        # 杀父进程，子进程会跟着退
# 或按端口找 PID 杀（WSL 里没有 netstat/ss 的话用 /proc）：
# pgrep -a -f server | grep -v vscode
```

---

## 3. 冒烟测试脚本（一条命令跑全链路）

已在 workspace 验证过，全通：

```bash
# 13 步冒烟：health → 实例 → 工具 → agent → MCP → 热更新 → 落盘 → 转发失败 → 非法实例
bash docs/bridge-smoke.sh

# SSE 变更广播：订阅 /mcp/:id → 触发变更 → 收 notifications/tools/list_changed
bash docs/bridge-sse-test.sh
```

> 脚本里服务地址写死 `127.0.0.1:18080`、key 写死 `testkey`，按需改脚本头部。
> 跑之前先启动服务（`ADDR=:18080 UE5_API_KEY=testkey`）。

---

## 4. 管理接口（UE5 插件调用方，`X-UE5-Key` 鉴权）

```bash
B=http://127.0.0.1:8080/api/v1/ue5
CT='Content-Type: application/json'
K='X-UE5-Key: testkey'

# ── 实例 ──
curl -s -X POST -H "$CT" -H "$K" $B/instances \
  -d '{"id":"demo","default_endpoint":"http://127.0.0.1:9999"}'
curl -s -H "$K" $B/instances                          # 列表
curl -s -H "$K" $B/instances/demo                     # 单个
curl -s -X DELETE -H "$K" $B/instances/demo           # 删除

# ── 工具（批量/单个/查/删） ──
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/tools -d '{"tools":[
  {"name":"look_inventory","description":"查看玩家背包","parameters":{"type":"object",
   "properties":{"player_id":{"type":"string","description":"玩家ID"}},"required":["player_id"]}},
  {"name":"get_time","description":"获取游戏内当前时间"}]}'
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/tools/look_inventory \
  -d '{"name":"look_inventory","description":"查看玩家背包"}'
curl -s -H "$K" $B/instances/demo/tools               # 列表
curl -s -H "$K" $B/instances/demo/tools/get_time      # 单个
curl -s -X DELETE -H "$K" $B/instances/demo/tools/get_time

# ── agent（⚠️ 批量注册有 bug，见第 8 节，用单发） ──
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/agents/npc_alice -d '{
  "name":"npc_alice","type":"actor",
  "system_prompt":"你是小镇杂货店老板爱丽丝。",
  "tools":["get_time"]}'
curl -s -H "$K" $B/instances/demo/agents              # 列表
curl -s -H "$K" $B/instances/demo/agents/npc_alice    # 单个
curl -s -X DELETE -H "$K" $B/instances/demo/agents/npc_alice
```

---

## 5. 热更新场景（核心！「靠近才给看背包的 tool」）

```bash
B=http://127.0.0.1:8080/api/v1/ue5
CT='Content-Type: application/json'; K='X-UE5-Key: testkey'

# 1. 注册工具（先注册，agent 引用时才能通过校验）
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/tools -d '{"tools":[
  {"name":"look_inventory","description":"查看玩家背包"},
  {"name":"get_time","description":"获取当前时间"}]}'

# 2. agent 初始只有 get_time（玩家不在附近）
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/agents/npc_alice -d '{
  "name":"npc_alice","type":"actor","system_prompt":"你是杂货店老板爱丽丝。",
  "tools":["get_time"]}'

# 3. 玩家靠近 → 热更新：tools 换成 [look_inventory]，prompt 同步更新
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/agents/npc_alice -d '{
  "name":"npc_alice","type":"actor","system_prompt":"你是杂货店老板爱丽丝。玩家就在你面前。",
  "tools":["look_inventory"]}'

# 4. 验证：MCP prompts/get 立即反映新 prompt + 新工具
curl -s -X POST http://127.0.0.1:8080/mcp/demo -d '{"jsonrpc":"2.0","id":"1","method":"prompts/get","params":{"name":"npc_alice"}}'
# → system 文本末尾 "- 你可用工具: look_inventory"

# 5. 玩家离开 → 再热更新回去（把 look_inventory 摘掉）
```

> 原理：`ChatHandler` 每次请求都 syncAgent（`SetSystemPrompt`/`SetTools`），`Runner` 每轮实时读 `agent.Tools` 无缓存，**下次对话立刻生效**，无需重启。

---

## 6. 对话接口（`/api/chat`，SSE 流式，需要 DEEPSEEK_API_KEY）

```bash
curl -N -X POST http://127.0.0.1:8080/api/chat \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: chatkey' \
  -d '{"instance_id":"demo","agent":"npc_alice","session_id":"player_1","message":"你好，我想看看我的背包"}'
```

SSE 事件类型：`text`（增量）· `tool_call`（工具调用）· `commentary`（推理/旁白）· `done`（结束，含 reply/usage）· `error`

请求体字段：`instance_id` / `agent` / `session_id`（会话隔离，同 ID 共享历史）/ `message`

---

## 7. MCP 协议（`/mcp/:instance_id`，JSON-RPC 2.0）

```bash
M=http://127.0.0.1:8080/mcp/demo

curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"1","method":"initialize"}'                       # 握手
curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"2","method":"ping"}'                             # 存活
curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"3","method":"tools/list"}'                       # 工具清单
curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"4","method":"tools/call","params":{"name":"get_time","arguments":{}}}'  # 调工具（转发 UE5 执行）
curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"5","method":"prompts/list"}'                     # NPC 列表
curl -s -X POST $M -d '{"jsonrpc":"2.0","id":"6","method":"prompts/get","params":{"name":"npc_alice"}}'  # 角色 prompt

# SSE 订阅变更（list_changed 广播，另开终端触发一次工具/agent 变更即可收到）
curl -N -s http://127.0.0.1:8080/mcp/demo
# → event: message / data: {"jsonrpc":"2.0","method":"notifications/tools/list_changed"}
```

支持批量请求（body 以 `[` 开头的 JSON 数组）。

---

## 8. ⚠️ 已知问题（2026-08-04 冒烟发现）

### Bug：批量注册 agent 必失败

- **现象**：`POST /instances/:id/agents`（批量）注册引用已有工具的 agent 时，报 `agent "xxx" 引用的 tool "yyy" 未在实例中注册`；但单发接口 `POST /instances/:id/agents/:name` 正常。
- **原因**：`internal/backend/manager.go` 的 `UpsertAgents` 事务校验副本 `dry := NewInstance(inst.ID, inst.DefaultEndpoint)` 是**全新空实例**，没有继承 `inst.tools`，dry-run 校验"引用的 tool 必须已注册"时必然找不到 → 批量注册 agent 100% 失败。
- **范围**：`UpsertTools` 批量没问题（`upsertTool` 不校验外部依赖）；`UpsertAgent`（单发）也没问题。
- **建议修复**：dry 副本继承实例已有内容：
  ```go
  dry := NewInstance(inst.ID, inst.DefaultEndpoint)
  for name, reg := range inst.tools { dry.tools[name] = reg }
  for name, def := range inst.agents { dry.agents[name] = def }
  ```
- **规避**：现阶段一律用单发接口注册/更新 agent。

### 其他注意点

- WSL 里 `go run` 的后台进程在会话关闭时收 SIGHUP 退出 → 启动必须 `nohup ... & disown`。
- `pkill -f 'exe/server'` 匹配不到编译产物（路径在 `.cache/go-build/.../server`，无 `exe/` 段），用 `pkill -f 'go run ./cmd/server'` 杀父进程。
- 工具执行转发失败会正常返回 JSON-RPC 错误（`-32603`），UE5 端没起时这是预期行为，不是 bug。

---

## 9. 清理

```bash
pkill -f 'go run ./cmd/server'
rm -rf /tmp/reg /tmp/reg-smoke /tmp/bridge*.log /tmp/sse-out.txt   # 测试残留
# 项目内 registry 目录（如果用了默认路径）
rm -rf /home/rechenz/projects/TheDemiugeAgent/TheDemiuge-Bridge/registry
```
