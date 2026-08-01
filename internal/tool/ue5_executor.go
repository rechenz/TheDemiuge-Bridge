// Package tool 实现 Agent 的工具执行层。
//
// ToolExecutor 是 ReAct 循环与具体工具实现的解耦点:
//   - UE5Executor:把模型发起的工具调用转发到 UE5 侧执行(游戏内动画/通信等),
//     是默认实现,绑定一个 UE5 实例;
//   - 本地工具(如 get_time)未来通过 FromStruct 反射注册,实现同一接口即可。
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// UE5Executor 把工具调用转发到 UE5 实例执行。
// 绑定单个 UE5 实例:工具查找、地址解析与转发都局限在该实例内。
type UE5Executor struct {
	mgr  *ue5.Manager
	cli  *ue5.Client
	inst *ue5.Instance
}

// NewUE5Executor 构造绑定指定 UE5 实例的工具执行器。
// inst 为空时,工具查找仍走 Manager(按实例 ID);转发地址解析由 cli 完成。
func NewUE5Executor(mgr *ue5.Manager, cli *ue5.Client, inst *ue5.Instance) *UE5Executor {
	return &UE5Executor{mgr: mgr, cli: cli, inst: inst}
}

// Execute 执行一次工具调用:从注册空间取工具定义 → 解析参数 → 转发 UE5 → 返回结果文本。
// 工具未注册时返回错误(ReAct 循环会作为错误结果回馈模型)。
func (e *UE5Executor) Execute(ctx context.Context, call types.ToolCall) (string, error) {
	instanceID := ""
	if e.inst != nil {
		instanceID = e.inst.ID
	}

	// 定位工具注册条目
	reg, ok := e.mgr.GetTool(instanceID, call.Function.Name)
	if !ok {
		return "", fmt.Errorf("工具 %q 未在实例 %q 中注册", call.Function.Name, instanceID)
	}

	// 解析参数:模型可能生成非法 JSON,解析失败按空参数处理并回馈
	args := map[string]any{}
	if call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("工具 %q 参数解析失败: %v", call.Function.Name, err)
		}
	}

	inst := e.inst
	if inst == nil {
		var ok bool
		inst, ok = e.mgr.GetInstance(instanceID)
		if !ok {
			return "", fmt.Errorf("实例 %q 不存在", instanceID)
		}
	}

	resp, err := e.cli.Forward(ctx, inst, reg, args)
	if err != nil {
		return "", err
	}
	return resp.Text()
}
