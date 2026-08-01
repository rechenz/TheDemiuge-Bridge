package ue5

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ── 落盘专用 DTO ────────────────────────────────────────────────────────────

// agentsFile 对应 {instance}/agents.yaml 的根结构。
type agentsFile struct {
	Agents []AgentDef `yaml:"agents" json:"agents"`
}

// toolsFile 对应 {instance}/tools.yaml 的根结构。
type toolsFile struct {
	Tools []ToolReg `yaml:"tools" json:"tools"`
}

// ── 变更通知 ────────────────────────────────────────────────────────────────

// ChangeKind 一次注册变更的种类。
type ChangeKind string

const (
	// ChangeTool 工具变更(tools/list_changed 广播)
	ChangeTool ChangeKind = "tool"
	// ChangeAgent agent 变更(agents/list_changed 广播)
	ChangeAgent ChangeKind = "agent"
)

// Change 一次注册变更信息。
// Kind 为变更种类,InstanceID 与 Name 定位被变更的条目。
type Change struct {
	Kind       ChangeKind `json:"kind"`
	InstanceID string     `json:"instance_id"`
	Name       string     `json:"name"`
}

// ChangeListener 注册变更回调。
// Manager 每次 tool/agent 注册或删除后调用,用于广播
// tools/list_changed / agents/list_changed 通知。
// 回调在 Manager 锁内同步执行;必须立即返回,不得阻塞。
type ChangeListener func(Change)

// ── Manager ─────────────────────────────────────────────────────────────────

// Manager 统一管理全部 UE5 实例的注册空间。
// 所有读写操作由内部互斥锁串行化,保证并发安全。
type Manager struct {
	mu        sync.Mutex
	instances map[string]*Instance

	dir      string
	listener ChangeListener
}

// ManagerOption 是 NewManager 的函数式选项。
type ManagerOption func(*Manager)

// WithRegistryDir 设置注册信息的落盘目录(默认 ./registry)。
// 目录不存在时自动创建;实例注册后实时写盘,重启时自动恢复。
func WithRegistryDir(dir string) ManagerOption {
	return func(m *Manager) { m.dir = dir }
}

// WithChangeListener 设置注册变更回调(变更广播通知)。
func WithChangeListener(l ChangeListener) ManagerOption {
	return func(m *Manager) { m.listener = l }
}

// NewManager 构造实例管理器。
// opts 可省略。注册目录默认 ./registry,变更回调默认 nil(不通知)。
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		instances: make(map[string]*Instance),
		dir:       "./registry",
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ── 实例管理 ────────────────────────────────────────────────────────────────

// RegisterInstance 注册一个实例(幂等)。
// 已存在时返回该实例(不重置其注册内容)。
func (m *Manager) RegisterInstance(id, defaultEndpoint string) *Instance {
	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, ok := m.instances[id]; ok {
		return inst
	}
	inst := NewInstance(id, defaultEndpoint)
	m.instances[id] = inst
	_ = m.persistInstanceLocked(inst)
	return inst
}

// GetInstance 按 ID 获取实例;不存在时返回 (nil, false)。
func (m *Manager) GetInstance(id string) (*Instance, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[id]
	return inst, ok
}

// RemoveInstance 注销实例,返回是否存在。
func (m *Manager) RemoveInstance(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.instances[id]; !ok {
		return false
	}
	delete(m.instances, id)
	_ = os.RemoveAll(m.instanceDirLocked(id))
	return true
}

// InstancesInfo 返回全部实例的概要信息列表(按 ID 排序)。
func (m *Manager) InstancesInfo() []InstanceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]InstanceInfo, 0, len(m.instances))
	for _, inst := range m.instances {
		out = append(out, inst.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InstanceIDs 返回已注册实例 ID 列表(按 ID 排序)。
func (m *Manager) InstanceIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]string, 0, len(m.instances))
	for id := range m.instances {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ── 工具操作(线程安全)──────────────────────────────────────────────────────

// UpsertTool 注册或覆盖一个工具。
// 实例不存在时自动创建(并继承 defaultEndpoint 为空)。
// 成功后回调变更通知并落盘。
func (m *Manager) UpsertTool(instanceID string, reg ToolReg) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst := m.instances[instanceID]
	if inst == nil {
		inst = m.RegisterInstance(instanceID, "")
	}
	if err := inst.upsertTool(reg); err != nil {
		return err
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		m.listener(Change{Kind: ChangeTool, InstanceID: instanceID, Name: reg.Name})
	}
	return nil
}

// UpsertTools 批量注册工具(事务语义:先在副本上全部校验,通过后才写回)。
func (m *Manager) UpsertTools(instanceID string, regs []ToolReg) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst := m.instances[instanceID]
	if inst == nil {
		inst = m.RegisterInstance(instanceID, "")
	}

	// 先在校验副本上演练,全部通过后统一写回真实注册空间
	dry := NewInstance(inst.ID, inst.DefaultEndpoint)
	for _, reg := range regs {
		if err := dry.upsertTool(reg); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(regs))
	for _, reg := range regs {
		_ = inst.upsertTool(reg)
		names = append(names, reg.Name)
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		for _, name := range names {
			m.listener(Change{Kind: ChangeTool, InstanceID: instanceID, Name: name})
		}
	}
	return nil
}

// DeleteTool 删除一个工具,返回是否存在。
func (m *Manager) DeleteTool(instanceID, name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst := m.instances[instanceID]
	if inst == nil {
		return false
	}
	if !inst.deleteTool(name) {
		return false
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		m.listener(Change{Kind: ChangeTool, InstanceID: instanceID, Name: name})
	}
	return true
}

// GetTool 查询工具注册条目。
func (m *Manager) GetTool(instanceID, name string) (ToolReg, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[instanceID]
	if !ok {
		return ToolReg{}, false
	}
	return inst.getTool(name)
}

// Tools 返回实例全部工具注册条目。
func (m *Manager) Tools(instanceID string) []ToolReg {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[instanceID]
	if !ok {
		return nil
	}
	return inst.toolsSnapshot()
}

// ── Agent 操作(线程安全)────────────────────────────────────────────────────

// UpsertAgent 注册或覆盖一个 agent。
// 引用的工具必须已存在于该实例。
func (m *Manager) UpsertAgent(instanceID string, def AgentDef) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.upsertAgentLocked(instanceID, def)
}

// UpsertAgents 批量注册 agent(事务语义:先在副本上全部校验,通过后才写回)。
func (m *Manager) UpsertAgents(instanceID string, defs []AgentDef) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst := m.instances[instanceID]
	if inst == nil {
		inst = m.RegisterInstance(instanceID, "")
	}

	// 先在校验副本上演练,全部通过后统一写回真实注册空间
	dry := NewInstance(inst.ID, inst.DefaultEndpoint)
	for _, def := range defs {
		if err := dry.upsertAgent(def); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		_ = inst.upsertAgent(def)
		names = append(names, def.Name)
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		for _, name := range names {
			m.listener(Change{Kind: ChangeAgent, InstanceID: instanceID, Name: name})
		}
	}
	return nil
}

// DeleteAgent 删除一个 agent,返回是否存在。
func (m *Manager) DeleteAgent(instanceID, name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst := m.instances[instanceID]
	if inst == nil {
		return false
	}
	if !inst.deleteAgent(name) {
		return false
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		m.listener(Change{Kind: ChangeAgent, InstanceID: instanceID, Name: name})
	}
	return true
}

// GetAgent 查询 agent 定义。
func (m *Manager) GetAgent(instanceID, name string) (AgentDef, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[instanceID]
	if !ok {
		return AgentDef{}, false
	}
	return inst.getAgent(name)
}

// Agents 返回实例全部 agent 定义。
func (m *Manager) Agents(instanceID string) []AgentDef {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[instanceID]
	if !ok {
		return nil
	}
	return inst.agentsSnapshot()
}

// ── 内部辅助 ────────────────────────────────────────────────────────────────

// upsertAgentLocked 在锁内注册 agent,并处理已存在实例的默认端点继承。
func (m *Manager) upsertAgentLocked(instanceID string, def AgentDef) error {
	inst := m.instances[instanceID]
	if inst == nil {
		inst = m.RegisterInstance(instanceID, "")
	}
	if err := inst.upsertAgent(def); err != nil {
		return err
	}
	_ = m.persistInstanceLocked(inst)
	if m.listener != nil {
		m.listener(Change{Kind: ChangeAgent, InstanceID: instanceID, Name: def.Name})
	}
	return nil
}

// instanceDirLocked 返回实例的落盘目录(调用方需持有锁)。
func (m *Manager) instanceDirLocked(instanceID string) string {
	return filepath.Join(m.dir, instanceID)
}

// persistInstanceLocked 将实例的 agent/tool 注册写入磁盘。
// 单文件方式:agents.yaml + tools.yaml 双文件,覆盖全量注册。
// 写盘失败不返回错误(注册已生效,仅持久化滞后),由管理者决定是否告警。
func (m *Manager) persistInstanceLocked(inst *Instance) error {
	dir := m.instanceDirLocked(inst.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建实例目录失败: %w", err)
	}

	// 写入 tools.yaml(工具注册清单)
	toolsData, err := yaml.Marshal(toolsFile{Tools: inst.toolsSnapshot()})
	if err != nil {
		return fmt.Errorf("marshal tools 失败: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(dir, "tools.yaml"), toolsData); err != nil {
		return err
	}

	// 写入 agents.yaml(agent 注册清单)
	agentsData, err := yaml.Marshal(agentsFile{Agents: inst.agentsSnapshot()})
	if err != nil {
		return fmt.Errorf("marshal agents 失败: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(dir, "agents.yaml"), agentsData); err != nil {
		return err
	}
	return nil
}

// writeFileAtomic 先写临时文件再原子重命名,避免写一半损坏。
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	return os.Rename(tmp, path)
}

// Restore 从落盘目录恢复所有实例。
// 扫描 m.dir 下的全部子目录,每个子目录对应一个实例,
// 读取 agents.yaml + tools.yaml 重建注册空间。
// 目录不存在时静默返回;恢复失败返回错误。
func (m *Manager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取注册目录 %s 失败: %w", m.dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		instanceID := entry.Name()
		if instanceID == "" || strings.HasPrefix(instanceID, ".") {
			continue
		}
		if err := m.restoreInstance(instanceID); err != nil {
			return fmt.Errorf("恢复实例 %q 失败: %w", instanceID, err)
		}
	}
	return nil
}

// restoreInstance 从磁盘恢复单个实例(调用方需持有锁)。
func (m *Manager) restoreInstance(instanceID string) error {
	dir := m.instanceDirLocked(instanceID)
	inst := NewInstance(instanceID, "")

	// 读取 tools.yaml
	var tf toolsFile
	if data, err := os.ReadFile(filepath.Join(dir, "tools.yaml")); err == nil {
		if err := yaml.Unmarshal(data, &tf); err != nil {
			return fmt.Errorf("解析 tools.yaml 失败: %w", err)
		}
		for _, reg := range tf.Tools {
			if err := inst.upsertTool(reg); err != nil {
				return fmt.Errorf("恢复 tool %q 失败: %w", reg.Name, err)
			}
		}
	}

	// 读取 agents.yaml
	var af agentsFile
	if data, err := os.ReadFile(filepath.Join(dir, "agents.yaml")); err == nil {
		if err := yaml.Unmarshal(data, &af); err != nil {
			return fmt.Errorf("解析 agents.yaml 失败: %w", err)
		}
		for _, def := range af.Agents {
			if err := inst.upsertAgent(def); err != nil {
				return fmt.Errorf("恢复 agent %q 失败: %w", def.Name, err)
			}
		}
	}

	m.instances[instanceID] = inst
	return nil
}
