package agent

type Message struct {
	Role    string `json:"role"` //system/user/assitant/tool
	Content string `json:"content"`
}

type ToolCall struct {
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"args"`
	Result string                 `json:"result,omitempty"`
}

type State struct {
	Messages  []Message //对话上下文
	MaxRounds int       //ReAct最大轮次
	Round     int       //当前轮次
}

func NewState(maxrounds int) *State {
	return &State{
		Messages: []Message{
			{Role: "system", Content: "你是一个智能NPC，请用角色的方式与玩家对话。"},
		},
		MaxRounds: maxrounds,
		Round:     0,
	}
}
