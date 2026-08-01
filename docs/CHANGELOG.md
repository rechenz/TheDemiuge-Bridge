# 工程日志

> 按日期倒序。记录架构决策、里程碑与踩坑。

---

## 2026-08-01（17:20）— MCP 后端解耦 + 命名 backend 化

### 完成

1. **MCP 协议层与后端彻底解耦（62a0a2a）**
   - 新增 `internal/mcp/registry.go`：`Registry` 通用接口
     - 数据面：`Tools` / `GetTool` / `Agents` / `GetAgent`（实时读取，`(val, bool)` 双返回）
     - 执行面：`ExecuteTool(ctx, instanceID, name, args) (string, error)`
     - 通用类型：`RegisteredAgent`（协议层 agent 描述）、`Change`/`ChangeKind`（从 ue5 上移）
   - 新增 `internal/backend/registry_adapter.go`：`RegistryAdapter` 把 `Manager + Client` 实现为 `mcp.Registry`
   - `mcp.Server` / `handler.ChatHandler` 只依赖 `Registry` 接口——**mcp 包不再 import ue5（依赖方向反转）**
   - chat.go 新增 `registryExecutor`（实现 `agent.ToolExecutor`），取代被删的 `tool.UE5Executor`
   - `internal/mcp/hub.go`：`OnChange` 用 mcp.Change（不再依赖 ue5 类型）

2. **删除（YAGNI 清理）**
   - `internal/tool/`（ue5_executor.go + 测试）
   - `internal/registry/`（decode.go / load.go / load_test.go）——YAML 静态注册废弃
   - `config/agents.yaml` / `config/tools.yaml`、`config.RegistryConfig`（AGENTS_FILE/TOOLS_FILE）
   - `types.ToolRegistry` / `types.AgentRegistry`、`ue5.FromTool` 反向转换链

3. **命名统一（2160ec9）**
   - `internal/ue5` → `internal/backend`，包名 `ue5` → `backend`，注释通用化（"UE5"→"后端"）
   - 保留：`ue5_handler.go` 文件名、`/api/v1/ue5/*`、`X-UE5-Key`、`UE5Config`、`UE5_API_KEY` 环境变量

### 架构收益

- MCP 协议层与任何后端解耦——接本地工具/其他引擎只需新写一个 Registry 实现
- 依赖方向：`mcp ← backend`（backend 复用 mcp 的类型），mcp 保持纯协议
- 单测仍全绿（chat 集成测试/热更新/历史窗口/hub 竞态/超时/message role/ID 校验）

### 下一步

- [ ] 真实 DEEPSEEK_API_KEY 端到端联调
- [ ] UE5 端 MCP Server 实现（照 docs/UE5-MCP-SERVER.md）

---

## 2026-08-01 — P0 核心对话跑通 + MCP 迁 UE5 决策

### 完成

1. **ReAct 循环接线（Bridge 自己跑对话）**
   - 新增 `internal/tool/ue5_executor.go`：`UE5Executor` 实现 `agent.ToolExecutor`，
     工具调用转发到 UE5 实例进程内执行（地址解析：工具 → 实例 → 全局默认）
   - 新增 `internal/server/handler/chat.go`：`POST /api/chat` SSE 流式对话
     - 事件协议：`text`（增量）/ `tool_call` / `commentary`（推理+旁白）/ `done`（reply+usage）/ `error`
     - `X-API-Key` 鉴权（`CHAT_API_KEY` 环境变量，空则不鉴权）
   - `cmd/server/main.go` 接线：DeepSeek Provider + UE5Executor + ChatHandler + MCP 共存
   - `internal/config` 新增 `ChatAPIKey` 字段

2. **测试补全（全绿）**
   - `internal/agent/react_test.go`：ReAct 循环 4 个单测（工具轮 / 单轮 / 最大轮次 / 无 executor）
   - `internal/tool/ue5_executor_test.go`：转发 4 个单测（结构化 / 文本 / 未注册 / UE5 500）
   - `internal/server/handler/chat_integration_test.go`：**全链路集成测试**——
     真实 Hertz 服务 + mock UE5 + mock LLM，验证 注册 → /api/chat → ReAct → 工具转发 → SSE 回推

3. **架构决策：MCP Server 迁往 UE5 端**
   - 分工：UE5 = 工具执行 + MCP Server（进程内零转发）；Bridge = ReAct 对话 + 注册中心
   - Go 侧 `internal/mcp/` 保留作协议参考/调试入口，不再扩展
   - 产出教程 `docs/UE5-MCP-SERVER.md`（协议规格 + C++ 骨架 + 传输选型）

4. **文档更新**
   - `docs/TODO.md`：P0 全勾；新增 P1「MCP 迁往 UE5 端」任务清单
   - `docs/ARCHITECTURE.md`：最终架构图 + 状态表刷新 + 各模块完成标记

### 踩坑

- `types.ChatResponse.Usage` 是值类型，mock 时不能取指针
- `ue5.toolParams` 是包私有类型，外部测试只能走 JSON 反序列化构造 ToolDef
- Hertz `internal/testutils` 不对外，集成测试用标准 `net.Listen("127.0.0.1:0")` 拿随机端口
- UE5 `ForwardResponse` 会把整个响应体（含 `{"result":...}` 壳）作为结构化结果，断言时注意

### 下一步

- [ ] commit 当前工作区（MCP 改造 + ReAct 接线一批）
- [ ] 真实 DEEPSEEK_API_KEY 端到端联调
- [ ] UE5 端 MCP Server 实现（照 docs/UE5-MCP-SERVER.md）

### 2026-08-01 补充（15:46）

- 调研 UE 社区 MCP 插件：确认两类（编辑器自动化 vs 游戏内 LLM 集成），
  结论：不整体替换，参考协议实现
- 存档 ChiR24/Unreal_mcp（807★，浅克隆）→ F:\\project\\TheDemiugeAgent\\references\\Unreal_mcp
  （含 README-REFERENCE.md 索引；C++ 传输层在 McpAutomationBridge 插件）
- 待办（之后做）：提炼 C++ 传输层可复用片段；对比 tool schema 兼容性
