package backend

import (
	"context"
	"fmt"

	"github.com/rechenz/TheDemiuge-Bridge/internal/mcp"
	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// RegistryAdapter 把 backend.Manager(注册中心) + backend.Client(工具执行) 组合,
// 实现 mcp.Registry 通用接口。MCP 协议层只依赖 Registry,
// "接入哪个后端、如何执行"由该适配器决定。
type RegistryAdapter struct {
	mgr *Manager
	cli *Client
}

// NewRegistryAdapter 构造适配器。
// mgr 提供注册数据读取;cli 提供工具执行(HTTP 转发到后端)。
func NewRegistryAdapter(mgr *Manager, cli *Client) *RegistryAdapter {
	return &RegistryAdapter{mgr: mgr, cli: cli}
}

// ── 数据面:从 Manager 注册空间实时读取 ─────────────────────────────────────

// Tools 返回实例全部工具(实时读取注册中心,无缓存)。
func (a *RegistryAdapter) Tools(instanceID string) ([]types.Tool, bool) {
	regs := a.mgr.Tools(instanceID)
	if regs == nil {
		return nil, false
	}
	out := make([]types.Tool, 0, len(regs))
	for _, reg := range regs {
		out = append(out, reg.ToTool())
	}
	return out, true
}

// GetTool 按名称取工具。
func (a *RegistryAdapter) GetTool(instanceID, name string) (types.Tool, bool) {
	reg, ok := a.mgr.GetTool(instanceID, name)
	if !ok {
		return types.Tool{}, false
	}
	return reg.ToTool(), true
}

// Agents 返回实例全部 agent。
func (a *RegistryAdapter) Agents(instanceID string) ([]mcp.RegisteredAgent, bool) {
	defs := a.mgr.Agents(instanceID)
	if defs == nil {
		return nil, false
	}
	out := make([]mcp.RegisteredAgent, 0, len(defs))
	for _, def := range defs {
		out = append(out, mcp.RegisteredAgent{
			Name:         def.Name,
			Type:         def.Type,
			SystemPrompt: def.SystemPrompt,
			ToolNames:    def.Tools,
		})
	}
	return out, true
}

// GetAgent 按名称取 agent。
func (a *RegistryAdapter) GetAgent(instanceID, name string) (mcp.RegisteredAgent, bool) {
	def, ok := a.mgr.GetAgent(instanceID, name)
	if !ok {
		return mcp.RegisteredAgent{}, false
	}
	return mcp.RegisteredAgent{
		Name:         def.Name,
		Type:         def.Type,
		SystemPrompt: def.SystemPrompt,
		ToolNames:    def.Tools,
	}, true
}

// ── 执行面:通过 Client 转发到后端 ──────────────────────────────────────────

// ExecuteTool 从注册空间取工具定义并转发后端执行。
func (a *RegistryAdapter) ExecuteTool(ctx context.Context, instanceID, name string, args map[string]any) (string, error) {
	inst, ok := a.mgr.GetInstance(instanceID)
	if !ok {
		return "", fmt.Errorf("实例 %q 不存在", instanceID)
	}
	reg, ok := a.mgr.GetTool(instanceID, name)
	if !ok {
		return "", fmt.Errorf("工具 %q 未在实例 %q 中注册", name, instanceID)
	}
	if args == nil {
		args = map[string]any{}
	}
	resp, err := a.cli.Forward(ctx, inst, reg, args)
	if err != nil {
		return "", err
	}
	return resp.Text()
}
