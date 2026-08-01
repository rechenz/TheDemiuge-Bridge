package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// LoadTools 从 YAML 文件加载全部 Tool 定义并注册到新的 ToolRegistry。
// 校验:Tool 名称非空且全局唯一(重复注册返回错误)。
func LoadTools(path string) (*types.ToolRegistry, error) {
	var file toolFile
	if err := decodeYAML(path, &file); err != nil {
		return nil, err
	}

	registry := types.NewToolRegistry()
	for _, def := range file.Tools {
		tool := def.toTool()
		if err := registry.Register(tool); err != nil {
			return nil, fmt.Errorf("tools.yaml 注册失败: %w", err)
		}
	}
	return registry, nil
}

// LoadAgents 从 YAML 文件加载全部 Agent 定义并注册到新的 AgentRegistry。
// agent 声明的 tools 通过引用名称绑定 toolRegistry 中已注册的 Tool,
// 引用不存在时返回错误。校验:Agent 名称非空且唯一、type 合法。
func LoadAgents(path string, toolRegistry *types.ToolRegistry) (*types.AgentRegistry, error) {
	var file agentFile
	if err := decodeYAML(path, &file); err != nil {
		return nil, err
	}

	registry := types.NewAgentRegistry()
	for _, def := range file.Agents {
		agent, err := def.toAgent(toolRegistry)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(agent); err != nil {
			return nil, fmt.Errorf("agents.yaml 注册失败: %w", err)
		}
	}
	return registry, nil
}

// decodeYAML 读取并解码 YAML 文件。
func decodeYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 YAML 文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("解析 YAML 文件 %s 失败: %w", path, err)
	}
	return nil
}

// ── DTO → 类型注册 ──────────────────────────────────────────────────────────

// toTool 将 YAML DTO 转换为请求侧 types.Tool。
func (d *toolDef) toTool() types.Tool {
	return types.Tool{
		Type: types.ToolTypeFunction,
		Function: types.Function{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters.toFunctionParameters(),
			Strict:      d.Strict,
		},
	}
}

// toAgent 将 YAML DTO 转换为 types.Agent,
// 通过名字引用绑定 toolRegistry 中已注册的 Tool。
func (d *agentDef) toAgent(toolRegistry *types.ToolRegistry) (*types.Agent, error) {
	agentType := types.AgentType(d.Type)
	switch agentType {
	case types.AgentTypeActor, types.AgentTypeSystem:
	default:
		return nil, fmt.Errorf("agents.yaml 中 Agent %q 的 type %q 非法,必须是 actor 或 system", d.Name, d.Type)
	}

	opts := []types.AgentOption{types.WithSystemPrompt(d.SystemPrompt)}
	if len(d.Tools) > 0 {
		tools := make([]types.Tool, 0, len(d.Tools))
		for _, name := range d.Tools {
			tool, ok := toolRegistry.Get(name)
			if !ok {
				return nil, fmt.Errorf("agents.yaml 中 Agent %q 引用的 tool %q 未注册", d.Name, name)
			}
			tools = append(tools, tool)
		}
		opts = append(opts, types.WithTools(tools...))
	}
	return types.NewAgent(d.Name, agentType, opts...), nil
}
