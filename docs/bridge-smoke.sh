#!/usr/bin/env bash
# TheDemiuge-Bridge 冒烟测试（管理接口 + MCP）
set -e
B=http://127.0.0.1:18080/api/v1/ue5
CT='Content-Type: application/json'
K='X-UE5-Key: testkey'

echo '== 0. 健康检查 =='
curl -s http://127.0.0.1:18080/api/v1/health
echo; echo '== 1. 注册实例 =='
curl -s -X POST -H "$CT" -H "$K" $B/instances -d '{"id":"demo","default_endpoint":"http://127.0.0.1:9999"}'
echo; echo '== 2. 批量注册工具 =='
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/tools -d '{"tools":[{"name":"look_inventory","description":"查看玩家背包内容","parameters":{"type":"object","properties":{"player_id":{"type":"string","description":"玩家ID"}},"required":["player_id"]}},{"name":"get_time","description":"获取游戏内当前时间"}]}'
echo; echo '== 3. 注册 agent（⚠️ 单发接口：批量接口有 bug 见文档） =='
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/agents/npc_alice -d '{"name":"npc_alice","type":"actor","system_prompt":"你是小镇杂货店老板爱丽丝。","tools":["get_time"]}'
echo; echo '== 4. 列出 agents =='
curl -s -H "$K" $B/instances/demo/agents
echo; echo '== 5. MCP tools/list =='
curl -s -X POST http://127.0.0.1:18080/mcp/demo -d '{"jsonrpc":"2.0","id":"1","method":"tools/list"}'
echo; echo '== 6. MCP prompts/get =='
curl -s -X POST http://127.0.0.1:18080/mcp/demo -d '{"jsonrpc":"2.0","id":"2","method":"prompts/get","params":{"name":"npc_alice"}}'
echo; echo '== 7. MCP ping =='
curl -s -X POST http://127.0.0.1:18080/mcp/demo -d '{"jsonrpc":"2.0","id":"3","method":"ping"}'
echo; echo '== 8. 热更新 agent（工具换成 look_inventory + 新 prompt） =='
curl -s -X POST -H "$CT" -H "$K" $B/instances/demo/agents/npc_alice -d '{"name":"npc_alice","type":"actor","system_prompt":"你是杂货店老板爱丽丝。玩家就在你面前。","tools":["look_inventory"]}'
echo; echo '== 9. 查询确认热更新 =='
curl -s -H "$K" $B/instances/demo/agents/npc_alice
echo; echo '== 10. MCP prompts/get 确认新 prompt =='
curl -s -X POST http://127.0.0.1:18080/mcp/demo -d '{"jsonrpc":"2.0","id":"4","method":"prompts/get","params":{"name":"npc_alice"}}'
echo; echo '== 11. 落盘文件 =='
ls /tmp/reg-smoke/demo/
echo; echo '== 12. 工具执行转发（endpoint 指向不存在 → 应报转发失败） =='
curl -s -X POST http://127.0.0.1:18080/mcp/demo -d '{"jsonrpc":"2.0","id":"5","method":"tools/call","params":{"name":"get_time","arguments":{}}}'
echo; echo '== 13. 非法实例 MCP 调用（应报错） =='
curl -s -X POST http://127.0.0.1:18080/mcp/nope -d '{"jsonrpc":"2.0","id":"6","method":"tools/list"}'
echo; echo '== DONE =='
