#!/usr/bin/env bash
# SSE 变更广播测试：订阅 /mcp/demo → 触发工具变更 → 应收到 notifications/tools/list_changed
set -e
B=http://127.0.0.1:18080
echo '== 订阅 SSE（后台 3 秒） =='
timeout 3 curl -N -s $B/mcp/demo > /tmp/sse-out.txt 2>&1 &
sleep 1
echo '== 触发工具变更 =='
curl -s -X POST -H 'Content-Type: application/json' -H 'X-UE5-Key: testkey' $B/api/v1/ue5/instances/demo/tools -d '{"tools":[{"name":"new_tool","description":"新增工具"}]}'
echo; echo '== SSE 收到的推送 =='
cat /tmp/sse-out.txt
echo '== DONE =='
