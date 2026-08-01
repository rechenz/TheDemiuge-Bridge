package ue5

import (
	"fmt"
	"strings"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// Instance 是一个 UE5 游戏服务器实例的注册空间。
// 每个实例持有独立的 tool 池与 agent 池,互不干扰。
// 并发安全由 Manager 串行化保证,本类型不内嵌锁。
type Instance struct {
	// ID 实例唯一标识(如游戏服名、进程 ID)
	ID string
	// DefaultEndpoint 实例级别的默认转发地址。
	// 工具未单独指定 Endpoint 时使用该地址执行转发。
	DefaultEndpoint string

	tools  map[string]ToolReg
	agents map[string]AgentDef
}

// NewInstance 构造一个空的实例注册空间。
func NewInstance(id, defaultEndpoint string) *Instance {
	return &Instance{
		ID:              id,
		DefaultEndpoint: defaultEndpoint,
		tools:           make(map[string]ToolReg),
		agents:          make(map[string]AgentDef),
	}
}

// ── Tool 操作 ───────────────────────────────────────────────────────────────

// upsertTool 注册或覆盖一个工具(热更新语义)。
// 校验通过后写回;名称非法或定义不合法时返回错误。
func (i *Instance) upsertTool(reg ToolReg) error {
	reg.Name = strings.TrimSpace(reg.Name)
	if reg.Name == "" {
		return fmt.Errorf("tool 名称不能为空")
	}
	if len(reg.Name) > 64 {
		return fmt.Errorf("tool 名称 %q 过长(最大 64 字符)", reg.Name)
	}
	if err := validateToolDef(reg.ToolDef); err != nil {
		return err
	}
	if reg.Endpoint != "" {
		reg.Endpoint = strings.TrimSpace(reg.Endpoint)
	}
	i.tools[reg.Name] = reg
	return nil
}

// deleteTool 移除一个工具,返回是否存在。
func (i *Instance) deleteTool(name string) bool {
	if _, ok := i.tools[name]; !ok {
		return false
	}
	delete(i.tools, name)
	return true
}

// getTool 按名称获取工具注册条目。
func (i *Instance) getTool(name string) (ToolReg, bool) {
	t, ok := i.tools[name]
	return t, ok
}

// toolsSnapshot 返回工具注册条目快照(顺序不保证)。
func (i *Instance) toolsSnapshot() []ToolReg {
	out := make([]ToolReg, 0, len(i.tools))
	for _, t := range i.tools {
		out = append(out, t)
	}
	return out
}

// ── Agent 操作 ──────────────────────────────────────────────────────────────

// upsertAgent 注册或覆盖一个 agent(热更新语义)。
// 校验:名称合法、type 为 actor/system、引用的工具必须存在于该实例。
func (i *Instance) upsertAgent(def AgentDef) error {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return fmt.Errorf("agent 名称不能为空")
	}
	switch types.AgentType(def.Type) {
	case types.AgentTypeActor, types.AgentTypeSystem:
	default:
		return fmt.Errorf("agent %q 的 type %q 非法,必须是 actor 或 system", def.Name, def.Type)
	}
	for _, toolName := range def.Tools {
		if _, ok := i.tools[toolName]; !ok {
			return fmt.Errorf("agent %q 引用的 tool %q 未在实例 %q 中注册", def.Name, toolName, i.ID)
		}
	}
	i.agents[def.Name] = def
	return nil
}

// deleteAgent 移除一个 agent,返回是否存在。
func (i *Instance) deleteAgent(name string) bool {
	if _, ok := i.agents[name]; !ok {
		return false
	}
	delete(i.agents, name)
	return true
}

// getAgent 按名称获取 agent 定义。
func (i *Instance) getAgent(name string) (AgentDef, bool) {
	a, ok := i.agents[name]
	return a, ok
}

// agentsSnapshot 返回 agent 定义快照(顺序不保证)。
func (i *Instance) agentsSnapshot() []AgentDef {
	out := make([]AgentDef, 0, len(i.agents))
	for _, a := range i.agents {
		out = append(out, a)
	}
	return out
}

// ── 概要 ────────────────────────────────────────────────────────────────────

// Info 返回实例概要信息。
func (i *Instance) Info() InstanceInfo {
	return InstanceInfo{
		ID:              i.ID,
		DefaultEndpoint: i.DefaultEndpoint,
		AgentCount:      len(i.agents),
		ToolCount:       len(i.tools),
	}
}

// validateToolDef 校验工具定义的基本合法性。
// parameters 出现时必须是 object 类型,且至少含 type 字段。
func validateToolDef(def ToolDef) error {
	if def.Name == "" {
		return fmt.Errorf("tool 名称不能为空")
	}
	if def.Parameters == nil {
		return nil
	}
	if def.Parameters.Type == "" {
		// 有 properties 时必须声明 type
		if len(def.Parameters.Properties) > 0 || len(def.Parameters.Required) > 0 {
			return fmt.Errorf("tool %q 的 parameters 缺少 type 字段", def.Name)
		}
	}
	if def.Parameters.Type != "" && def.Parameters.Type != types.SchemaTypeObject {
		return fmt.Errorf("tool %q 的 parameters.type 必须是 object,得到 %q", def.Name, def.Parameters.Type)
	}
	return nil
}
