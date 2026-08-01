package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// ── 辅助工具 ────────────────────────────────────────────────────────────────

// writeTempYAML 创建临时 YAML 文件并返回路径。
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入临时 YAML 失败: %v", err)
	}
	return path
}

// mustLoadTools 加载工具注册表,失败即终止测试。
func mustLoadTools(t *testing.T, content string) *types.ToolRegistry {
	t.Helper()
	path := writeTempYAML(t, content)
	r, err := LoadTools(path)
	if err != nil {
		t.Fatalf("LoadTools 失败: %v", err)
	}
	return r
}

// ── LoadTools ───────────────────────────────────────────────────────────────

func TestLoadTools_Success(t *testing.T) {
	r := mustLoadTools(t, `
tools:
  - name: get_time
    description: 获取当前时间
    parameters:
      type: object
      properties:
        timezone:
          type: string
          description: 时区名
      required:
        - timezone
  - name: get_weather
    description: 获取天气
    parameters:
      type: object
      properties:
        city:
          type: string
      required:
        - city
`)

	if got := len(r.All()); got != 2 {
		t.Fatalf("期望注册 2 个 tool,得到 %d", got)
	}

	tool, ok := r.Get("get_time")
	if !ok {
		t.Fatal("get_time 应已注册")
	}
	if tool.Function.Name != "get_time" {
		t.Errorf("tool 名称错误: %s", tool.Function.Name)
	}
	if tool.Type != types.ToolTypeFunction {
		t.Errorf("tool type 应为 %q,得到 %q", types.ToolTypeFunction, tool.Type)
	}
	if tool.Function.Parameters == nil {
		t.Fatal("get_time 应携带 parameters")
	}
	if _, ok := tool.Function.Parameters.Properties["timezone"]; !ok {
		t.Error("get_time 应包含 timezone 属性")
	}
}

func TestLoadTools_EmptyFile(t *testing.T) {
	r := mustLoadTools(t, ``)
	if got := len(r.All()); got != 0 {
		t.Fatalf("空文件应注册 0 个 tool,得到 %d", got)
	}
}

func TestLoadTools_DuplicateName(t *testing.T) {
	path := writeTempYAML(t, `
tools:
  - name: get_time
    description: 第一次
  - name: get_time
    description: 重复
`)
	_, err := LoadTools(path)
	if err == nil {
		t.Fatal("重复 tool 名称应返回错误")
	}
	if !strings.Contains(err.Error(), "已注册") {
		t.Errorf("错误信息应包含 '已注册',得到: %v", err)
	}
}

func TestLoadTools_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")
	_, err := LoadTools(path)
	if err == nil {
		t.Fatal("文件不存在应返回错误")
	}
	if !strings.Contains(err.Error(), "读取 YAML 文件") {
		t.Errorf("错误信息应包含 '读取 YAML 文件',得到: %v", err)
	}
}

func TestLoadTools_InvalidYAML(t *testing.T) {
	path := writeTempYAML(t, "tools: [unclosed")
	_, err := LoadTools(path)
	if err == nil {
		t.Fatal("非法 YAML 应返回错误")
	}
	if !strings.Contains(err.Error(), "解析 YAML 文件") {
		t.Errorf("错误信息应包含 '解析 YAML 文件',得到: %v", err)
	}
}

// ── LoadAgents ──────────────────────────────────────────────────────────────

// sampleToolsContent 供 LoadAgents 测试复用的工具定义。
const sampleToolsContent = `
tools:
  - name: get_time
    description: 获取当前时间
    parameters:
      type: object
      properties:
        timezone:
          type: string
      required:
        - timezone
  - name: get_weather
    description: 获取天气
`

func TestLoadAgents_Success(t *testing.T) {
	toolRegistry := mustLoadTools(t, sampleToolsContent)

	path := writeTempYAML(t, `
agents:
  - name: npc_alice
    type: actor
    system_prompt: |
      你是小镇居民艾丽丝。
    tools:
      - get_time
  - name: game_master
    type: system
    system_prompt: |
      你是游戏主持人。
    tools:
      - get_time
      - get_weather
`)
	r, err := LoadAgents(path, toolRegistry)
	if err != nil {
		t.Fatalf("LoadAgents 失败: %v", err)
	}

	if got := len(r.All()); got != 2 {
		t.Fatalf("期望注册 2 个 agent,得到 %d", got)
	}

	alice, ok := r.Get("npc_alice")
	if !ok {
		t.Fatal("npc_alice 应已注册")
	}
	if alice.Type != types.AgentTypeActor {
		t.Errorf("npc_alice type 应为 actor,得到 %s", alice.Type)
	}
	if len(alice.Tools) != 1 {
		t.Fatalf("npc_alice 应绑定 1 个 tool,得到 %d", len(alice.Tools))
	}
	if alice.Tools[0].Function.Name != "get_time" {
		t.Errorf("npc_alice 应绑定 get_time,得到 %s", alice.Tools[0].Function.Name)
	}

	gm, ok := r.Get("game_master")
	if !ok {
		t.Fatal("game_master 应已注册")
	}
	if gm.Type != types.AgentTypeSystem {
		t.Errorf("game_master type 应为 system,得到 %s", gm.Type)
	}
	if len(gm.Tools) != 2 {
		t.Fatalf("game_master 应绑定 2 个 tool,得到 %d", len(gm.Tools))
	}
}

func TestLoadAgents_NoTools(t *testing.T) {
	toolRegistry := mustLoadTools(t, sampleToolsContent)

	path := writeTempYAML(t, `
agents:
  - name: npc_bob
    type: actor
    system_prompt: |
      你是商人鲍勃。
`)
	r, err := LoadAgents(path, toolRegistry)
	if err != nil {
		t.Fatalf("LoadAgents 失败: %v", err)
	}

	bob, ok := r.Get("npc_bob")
	if !ok {
		t.Fatal("npc_bob 应已注册")
	}
	if len(bob.Tools) != 0 {
		t.Errorf("npc_bob 不应绑定 tool,得到 %d", len(bob.Tools))
	}
}

func TestLoadAgents_UnregisteredTool(t *testing.T) {
	toolRegistry := mustLoadTools(t, sampleToolsContent)

	path := writeTempYAML(t, `
agents:
  - name: npc_alice
    type: actor
    tools:
      - not_exist_tool
`)
	_, err := LoadAgents(path, toolRegistry)
	if err == nil {
		t.Fatal("引用未注册 tool 应返回错误")
	}
	if !strings.Contains(err.Error(), "未注册") {
		t.Errorf("错误信息应包含 '未注册',得到: %v", err)
	}
}

func TestLoadAgents_InvalidType(t *testing.T) {
	toolRegistry := mustLoadTools(t, sampleToolsContent)

	path := writeTempYAML(t, `
agents:
  - name: npc_alice
    type: robot
`)
	_, err := LoadAgents(path, toolRegistry)
	if err == nil {
		t.Fatal("非法 type 应返回错误")
	}
	if !strings.Contains(err.Error(), "必须是 actor 或 system") {
		t.Errorf("错误信息应包含 '必须是 actor 或 system',得到: %v", err)
	}
}

func TestLoadAgents_DuplicateName(t *testing.T) {
	toolRegistry := mustLoadTools(t, sampleToolsContent)

	path := writeTempYAML(t, `
agents:
  - name: npc_alice
    type: actor
  - name: npc_alice
    type: system
`)
	_, err := LoadAgents(path, toolRegistry)
	if err == nil {
		t.Fatal("重复 agent 名称应返回错误")
	}
	if !strings.Contains(err.Error(), "已注册") {
		t.Errorf("错误信息应包含 '已注册',得到: %v", err)
	}
}

// ── 嵌套 Schema 转换 ────────────────────────────────────────────────────────

func TestToTool_NestedSchema(t *testing.T) {
	path := writeTempYAML(t, `
tools:
  - name: place_order
    description: 下单
    parameters:
      type: object
      properties:
        items:
          type: array
          description: 商品列表
          items:
            type: object
            properties:
              sku:
                type: string
              qty:
                type: integer
            required:
              - sku
      required:
        - items
`)
	r, err := LoadTools(path)
	if err != nil {
		t.Fatalf("LoadTools 失败: %v", err)
	}

	tool, ok := r.Get("place_order")
	if !ok {
		t.Fatal("place_order 应已注册")
	}
	items := tool.Function.Parameters.Properties["items"]
	if items.Type != types.SchemaTypeArray {
		t.Errorf("items 类型应为 array,得到 %s", items.Type)
	}
	if items.Items == nil || items.Items.Type != types.SchemaTypeObject {
		t.Fatalf("items 应嵌套 object schema")
	}
	if _, ok := items.Items.Properties["sku"]; !ok {
		t.Error("嵌套 object 应包含 sku 属性")
	}
}
