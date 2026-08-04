package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateInstanceID 验证实例 ID 合法性规则。
func TestValidateInstanceID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"inst_a", true},
		{"instance-01", true},
		{"a.b.c", true},
		{"A1_b-2.c", true},
		{"", false},
		{"../etc/passwd", false},
		{"..", false},
		{".", false},
		{"a/b", false},
		{"a\\b", false},
		{"a b", false},
		{"a:b", false},
		{"a?b", false},
		{strings.Repeat("a", 65), false},
		{strings.Repeat("a", 64), true},
	}
	for _, c := range cases {
		if got := validateInstanceID(c.id); got != c.want {
			t.Errorf("validateInstanceID(%q) = %v,期望 %v", c.id, got, c.want)
		}
	}
}

// TestRegisterInstance_RejectTraversalID 验证含路径穿越字符的实例 ID 被拒绝,
// 且不会在注册目录下创建任何目录。
func TestRegisterInstance_RejectTraversalID(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(WithRegistryDir(dir))

	if inst := mgr.RegisterInstance("../evil", ""); inst != nil {
		t.Fatalf("路径穿越实例 ID 应被拒绝,实际返回 %q", inst.ID)
	}

	// 注册目录下不应生成任何子目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取注册目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("注册目录不应有任何内容,实际: %v", entries)
	}
}

// TestUpsertTool_RejectTraversalID 验证通过 UpsertTool 隐式创建实例时同样拒绝非法 ID。
func TestUpsertTool_RejectTraversalID(t *testing.T) {
	mgr := NewManager(WithRegistryDir(t.TempDir()))
	err := mgr.UpsertTool("../evil", ToolReg{ToolDef: ToolDef{Name: "tool_a"}})
	if err == nil {
		t.Fatal("UpsertTool 到非法实例 ID 应返回错误")
	}
	if !strings.Contains(err.Error(), "非法") {
		t.Errorf("错误信息应提到非法,实际: %v", err)
	}
}

// TestUpsertAgent_RejectTraversalID 验证通过 UpsertAgent 隐式创建实例时同样拒绝非法 ID。
func TestUpsertAgent_RejectTraversalID(t *testing.T) {
	mgr := NewManager(WithRegistryDir(t.TempDir()))
	err := mgr.UpsertAgent("../../x", AgentDef{Name: "npc", Type: "actor"})
	if err == nil {
		t.Fatal("UpsertAgent 到非法实例 ID 应返回错误")
	}
}

// TestRemoveInstance_AfterDeleteDirCleared 验证注销实例后落盘目录被清理。
func TestRemoveInstance_AfterDeleteDirCleared(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(WithRegistryDir(dir))
	inst := mgr.RegisterInstance("inst_a", "")
	if inst == nil {
		t.Fatal("注册实例失败")
	}

	if !mgr.RemoveInstance("inst_a") {
		t.Fatal("注销实例失败")
	}
	if _, err := os.Stat(filepath.Join(dir, "inst_a")); !os.IsNotExist(err) {
		t.Errorf("注销后实例目录应被删除,err = %v", err)
	}
}

// ── 批量注册(事务) ────────────────────────────────────────────────────────

// TestUpsertAgents_BatchWithToolRefs 回归测试:批量注册引用已注册工具的 agent 应成功。
// 曾因 dry-run 校验副本未继承实例已注册内容而误报"工具未注册"。
func TestUpsertAgents_BatchWithToolRefs(t *testing.T) {
	mgr := NewManager(WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", "")
	if err := mgr.UpsertTool(inst.ID, ToolReg{ToolDef: ToolDef{Name: "tool_a", Description: "工具A"}}); err != nil {
		t.Fatalf("注册 tool_a 失败: %v", err)
	}

	defs := []AgentDef{
		{Name: "npc_a", Type: "actor", SystemPrompt: "A", Tools: []string{"tool_a"}},
		{Name: "npc_b", Type: "actor", SystemPrompt: "B", Tools: []string{"tool_a"}},
	}
	if err := mgr.UpsertAgents(inst.ID, defs); err != nil {
		t.Fatalf("批量注册引用已注册工具的 agent 应成功,实际: %v", err)
	}
	if _, ok := mgr.GetAgent(inst.ID, "npc_a"); !ok {
		t.Error("npc_a 应已注册")
	}
	if _, ok := mgr.GetAgent(inst.ID, "npc_b"); !ok {
		t.Error("npc_b 应已注册")
	}
}

// TestUpsertAgents_RejectsUnknownTool 批量中引用未注册工具应报错,且无部分写入(事务原子性)。
func TestUpsertAgents_RejectsUnknownTool(t *testing.T) {
	mgr := NewManager(WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", "")
	if err := mgr.UpsertTool(inst.ID, ToolReg{ToolDef: ToolDef{Name: "tool_a"}}); err != nil {
		t.Fatalf("注册 tool_a 失败: %v", err)
	}
	// 预注册一个 agent:验证批量失败不影响已有内容(事务原子性)
	if err := mgr.UpsertAgent(inst.ID, AgentDef{Name: "npc_old", Type: "actor", Tools: []string{"tool_a"}}); err != nil {
		t.Fatalf("预注册 npc_old 失败: %v", err)
	}

	defs := []AgentDef{
		{Name: "npc_ok", Type: "actor", Tools: []string{"tool_a"}},
		{Name: "npc_bad", Type: "actor", Tools: []string{"ghost_tool"}},
	}
	err := mgr.UpsertAgents(inst.ID, defs)
	if err == nil {
		t.Fatal("引用未注册工具应返回错误")
	}
	if !strings.Contains(err.Error(), "ghost_tool") {
		t.Errorf("错误信息应包含未注册工具名,实际: %v", err)
	}
	if _, ok := mgr.GetAgent(inst.ID, "npc_ok"); ok {
		t.Error("批量失败后 npc_ok 不应被部分写入")
	}
	if _, ok := mgr.GetAgent(inst.ID, "npc_old"); !ok {
		t.Error("批量失败后已存在的 npc_old 应保留")
	}
}

// TestUpsertAgents_AutoCreateInstance 批量写入不存在的实例 ID 应隐式建实例并成功。
func TestUpsertAgents_AutoCreateInstance(t *testing.T) {
	mgr := NewManager(WithRegistryDir(t.TempDir()))
	if err := mgr.UpsertAgents("brand_new", []AgentDef{{Name: "npc_a", Type: "actor"}}); err != nil {
		t.Fatalf("批量注册到新实例应成功: %v", err)
	}
	if _, ok := mgr.GetAgent("brand_new", "npc_a"); !ok {
		t.Error("npc_a 应已注册到自动创建的实例")
	}
}
