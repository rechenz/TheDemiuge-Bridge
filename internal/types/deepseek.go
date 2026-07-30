package types

type DeepseekChatRequest struct {
	Messages `json:"messages"`
	Model    string `json:"model"`
	Thinking struct {
		Type string `json:"type"`
	} `json:"thinking"`
	Reasoning_Effort string `json:"reasoning_effort"`
	Max_tokens       int    `json:"max_tokens"`
	Response_Format  struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Stop           string `json:"stop"`
	Stream         bool   `json:"stream"`
	Stream_Options struct {
		Include_Usage bool `json:"include_usage"`
	} `json:"stream_options"`
	Temperature float32 `json:"temperature"`
	//tools未定义
}
