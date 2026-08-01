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
