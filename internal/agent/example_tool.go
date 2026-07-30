package agent

import (
	"time"
)

type GetTimeTool struct{}

func (t *GetTimeTool) Name() string {
	return "get_current_time"
}

func (t *GetTimeTool) Description() string {
	return "获取当前的日期和时间"
}

func (t *GetTimeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"format": map[string]interface{}{
				"type":        "string",
				"description": "时间格式，可选 date/time/datetime",
				"enum":        []string{"date", "time", "datetime"},
			},
		},
		"required": []string{},
	}
}

func (t *GetTimeTool) Execute(args map[string]interface{}) (string, error) {
	format, _ := args["format"].(string)
	switch format {
	case "date":
		return time.Now().Format("2006-01-02"), nil
	case "time":
		return time.Now().Format("15:04:05"), nil
	default:
		return time.Now().Format("2006-01-02 15:04:05"), nil
	}
}
