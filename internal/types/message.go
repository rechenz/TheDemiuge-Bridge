package types

type Message struct {
	Role         string `json:"role"`
	Tool_Call_Id string `json:"tool_call_id,omitempty"`
	Content      string `json:"content"`
}

type Messages struct {
	Messages []Message
}
