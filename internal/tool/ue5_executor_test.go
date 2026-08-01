package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// newTestEnv 构造 Manager + Client + 实例 + 工具注册,指向 httptest UE5 端。
func newTestEnv(t *testing.T, ue5Handler http.HandlerFunc) (*ue5.Manager, *ue5.Client, *ue5.Instance) {
	t.Helper()
	srv := httptest.NewServer(ue5Handler)
	t.Cleanup(srv.Close)

	mgr := ue5.NewManager(ue5.WithRegistryDir(t.TempDir()))
	inst := mgr.RegisterInstance("inst_a", srv.URL)
	_ = inst

	cli := &ue5.Client{HTTPClient: srv.Client()}
	return mgr, cli, inst
}

// registerTestTool 注册一个 play_animation 工具到实例。
// toolParams 为 ue5 包私有类型,外部通过 JSON 反序列化构造(与 UE5 管理接口一致)。
func registerTestTool(t *testing.T, mgr *ue5.Manager, inst *ue5.Instance) {
	t.Helper()
	raw := `{
		"name": "play_animation",
		"description": "让 NPC 播放指定动画",
		"parameters": {
			"type": "object",
			"properties": {"anim": {"type": "string", "description": "动画名"}},
			"required": ["anim"]
		}
	}`
	var reg ue5.ToolReg
	if err := json.Unmarshal([]byte(raw), &reg); err != nil {
		t.Fatalf("构造工具注册失败: %v", err)
	}
	if err := mgr.UpsertTool(inst.ID, reg); err != nil {
		t.Fatalf("注册工具失败: %v", err)
	}
}

func TestUE5Executor_Execute_ForwardAndParse(t *testing.T) {
	var gotName, gotArgs string
	ue5Srv := func(w http.ResponseWriter, r *http.Request) {
		var req ue5.ForwardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析转发请求失败: %v", err)
		}
		gotName = req.Name
		b, _ := json.Marshal(req.Arguments)
		gotArgs = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"success":true,"anim":"wave"}}`))
	}

	mgr, cli, inst := newTestEnv(t, ue5Srv)
	registerTestTool(t, mgr, inst)
	exec := NewUE5Executor(mgr, cli, inst)

	text, err := exec.Execute(context.Background(), types.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "play_animation",
			Arguments: `{"anim":"wave"}`,
		},
	})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if gotName != "play_animation" {
		t.Errorf("转发的工具名 = %q,期望 play_animation", gotName)
	}
	if gotArgs != `{"anim":"wave"}` {
		t.Errorf("转发的参数 = %s,期望 {\"anim\":\"wave\"}", gotArgs)
	}
	// Forward 把整个响应体(含 result 壳)解析为结构化结果,Text 原样输出
	want := `{"result":{"anim":"wave","success":true}}`
	if text != want {
		// map 序列化键序不定,解析后比较语义
		var gotMap, wantMap map[string]any
		_ = json.Unmarshal([]byte(text), &gotMap)
		_ = json.Unmarshal([]byte(want), &wantMap)
		if fmt.Sprintf("%v", gotMap) != fmt.Sprintf("%v", wantMap) {
			t.Errorf("结果文本 = %s,期望语义等价 %s", text, want)
		}
	}
}

func TestUE5Executor_Execute_TextResponse(t *testing.T) {
	ue5Srv := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("动画已播放"))
	}

	mgr, cli, inst := newTestEnv(t, ue5Srv)
	registerTestTool(t, mgr, inst)
	exec := NewUE5Executor(mgr, cli, inst)

	text, err := exec.Execute(context.Background(), types.ToolCall{
		Function: types.ToolCallFunction{Name: "play_animation", Arguments: `{}`},
	})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if text != "动画已播放" {
		t.Errorf("结果文本 = %q,期望 动画已播放", text)
	}
}

func TestUE5Executor_Execute_UnknownTool(t *testing.T) {
	mgr, cli, inst := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("不应转发未注册的工具")
	})

	exec := NewUE5Executor(mgr, cli, inst)
	_, err := exec.Execute(context.Background(), types.ToolCall{
		Function: types.ToolCallFunction{Name: "no_such_tool", Arguments: `{}`},
	})
	if err == nil {
		t.Fatal("未注册工具应返回错误")
	}
}

func TestUE5Executor_Execute_UE5Error(t *testing.T) {
	ue5Srv := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}

	mgr, cli, inst := newTestEnv(t, ue5Srv)
	registerTestTool(t, mgr, inst)
	exec := NewUE5Executor(mgr, cli, inst)

	_, err := exec.Execute(context.Background(), types.ToolCall{
		Function: types.ToolCallFunction{Name: "play_animation", Arguments: `{}`},
	})
	if err == nil {
		t.Fatal("UE5 返回 500 应返回错误")
	}
}
