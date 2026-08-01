package config

import (
	"crypto/tls"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rechenz/TheDemiuge-Bridge/internal/types"
)

// DeepSeekBaseURL DeepSeek 官方 Chat Completions 接口基地址。
const DeepSeekBaseURL = "https://api.deepseek.com/chat/completions"

// RegistryConfig 注册存储位置配置。
// Agent 与 Tool 的定义均通过 YAML 文件读写,由启动时加载注册。
type RegistryConfig struct {
	// AgentsFile Agent 定义 YAML 文件路径,默认 config/agents.yaml
	AgentsFile string
	// ToolsFile Tool 定义 YAML 文件路径,默认 config/tools.yaml
	ToolsFile string
}

// Config 服务运行配置。
type Config struct {
	// Addr 服务器监听地址
	Addr string
	// APIKey LLM 提供方 API Key
	APIKey string
	// Registry Agent / Tool 注册的 YAML 存储位置
	Registry RegistryConfig
	// BaseURL LLM 提供方 Chat Completions 接口地址,默认 DeepSeek 官方地址。
	// 换 OpenAI / 本地模型等兼容服务时只需改这里,上层零改动。
	BaseURL string
	// ModelName 模型名:deepseek-v4-flash 或 deepseek-v4-pro
	ModelName string
	// MaxTokens 限制一次请求中模型生成 completion 的最大 token 数
	MaxTokens int
	// Temperature 采样温度,介于 0 和 2 之间;建议与 TopP 二选一修改
	Temperature float32
	// TopP 核采样,介于 0 和 1 之间;建议与 Temperature 二选一修改
	TopP float32
	// Thinking 思考模式配置;nil 表示不发送,由服务端默认(enabled)
	Thinking *types.ThinkingConfig
	// ReasoningEffort 推理强度:low / high / max(默认 high)
	ReasoningEffort string
	// ResponseFormat 输出格式;nil 表示默认 text
	ResponseFormat *types.ResponseFormat
	// Stop 停止序列;nil 表示不设置
	Stop *types.StopSequence
	// UserID 自定义业务侧用户标识,用于内容安全与 KVCache 隔离
	UserID string
	// Stream 是否流式输出,默认 true
	Stream bool
	// StreamOptions 流式输出选项;仅 Stream 为 true 时有意义
	StreamOptions *types.StreamOptions
	// HTTPTimeout 单次 LLM HTTP 请求超时时间。0 表示不设超时。
	HTTPTimeout time.Duration
	// HTTPClient 可选注入的自定义 http.Client(如带 Transport 的测试客户端)。
	// nil 时使用带 HTTPTimeout 的默认客户端。
	HTTPClient *http.Client
}

// Load 从环境变量加载配置,缺失项使用默认值。
func Load() *Config {
	thinking := types.ThinkingEnabled
	return &Config{
		Addr:   getEnv("ADDR", ":8080"),
		APIKey: getEnv("DEEPSEEK_API_KEY", ""),
		Registry: RegistryConfig{
			AgentsFile: getEnv("AGENTS_FILE", "config/agents.yaml"),
			ToolsFile:  getEnv("TOOLS_FILE", "config/tools.yaml"),
		},
		BaseURL:         getEnv("DEEPSEEK_BASE_URL", DeepSeekBaseURL),
		ModelName:       getEnv("MODEL_NAME", types.ModelV4Flash),
		MaxTokens:       getEnvInt("MAX_TOKENS", 4096),
		Temperature:     getEnvFloat32("TEMPERATURE", 0.7),
		TopP:            getEnvFloat32("TOP_P", 1),
		Thinking:        &types.ThinkingConfig{Type: getEnv("THINKING", thinking)},
		ReasoningEffort: getEnv("REASONING_EFFORT", "high"),
		ResponseFormat:  newResponseFormat(getEnv("RESPONSE_FORMAT", types.ResponseFormatText)),
		Stop:            newStopSequence(getEnv("STOP", "")),
		UserID:          getEnv("USER_ID", ""),
		Stream:          getEnvBool("STREAM", true),
		StreamOptions:   newStreamOptions(getEnvBool("STREAM_INCLUDE_USAGE", false)),
		HTTPTimeout:     getEnvDuration("HTTP_TIMEOUT", 60*time.Second),
		HTTPClient:      newHTTPClient(getEnv("HTTP_CLIENT", "")),
	}
}

// newHTTPClient 根据环境变量构造自定义 http.Client。
// 当前仅支持 "insecure" —— 跳过 TLS 校验(本地 http mock 联调用);
// 其余值返回 nil(使用默认客户端)。
func newHTTPClient(v string) *http.Client {
	if v == "insecure" {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}
	return nil
}

// ToChatRequest 将配置映射为 Chat Completions 请求体。
// messages 为空时返回 nil,由调用方保证至少 1 条消息。
// opts 为单次对话可选参数:nil 或空字段不产生效果。
func (c *Config) ToChatRequest(messages []types.Message, opts *types.ChatOptions) *types.ChatRequest {
	if len(messages) == 0 || c == nil {
		return nil
	}

	req := &types.ChatRequest{
		Messages:        messages,
		Model:           c.ModelName,
		Thinking:        c.Thinking,
		ReasoningEffort: c.ReasoningEffort,
		ResponseFormat:  c.ResponseFormat,
		Stop:            c.Stop,
		Stream:          c.Stream,
		StreamOptions:   c.StreamOptions,
		UserID:          c.UserID,
	}

	// 单次覆盖项:非零值优先于 config
	if opts != nil {
		if len(opts.Tools) > 0 {
			req.Tools = opts.Tools
		}
		if opts.ToolChoice != nil {
			req.ToolChoice = opts.ToolChoice
		}
		if opts.Model != "" {
			req.Model = opts.Model
		}
		if opts.MaxTokens != nil {
			req.MaxTokens = opts.MaxTokens
		}
		if opts.Temperature != nil {
			req.Temperature = opts.Temperature
		}
		if opts.TopP != nil {
			req.TopP = opts.TopP
		}
	}

	if req.Model == "" {
		req.Model = c.ModelName
	}

	if req.MaxTokens == nil && c.MaxTokens > 0 {
		mt := c.MaxTokens
		req.MaxTokens = &mt
	}
	if req.Temperature == nil && c.Temperature > 0 {
		t := c.Temperature
		req.Temperature = &t
	}
	if req.TopP == nil && c.TopP > 0 {
		p := c.TopP
		req.TopP = &p
	}

	return req
}

// newResponseFormat 将环境变量值映射为 ResponseFormat;
// 非法值(非 text/json_object)回退为 text。
func newResponseFormat(v string) *types.ResponseFormat {
	switch v {
	case types.ResponseFormatText, types.ResponseFormatJSONObject:
		return &types.ResponseFormat{Type: v}
	default:
		return nil
	}
}

// newStopSequence 将逗号分隔的环境变量值映射为 StopSequence;
// 空串返回 nil,单个值返回单元素序列,多个值返回列表(最多 16 个)。
func newStopSequence(v string) *types.StopSequence {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 1 {
		return types.NewStopSequence(parts[0])
	}
	return types.NewStopSequenceList(parts...)
}

// newStreamOptions 根据 include usage 配置构造流式选项;
// 未开启时返回 nil。
func newStreamOptions(includeUsage bool) *types.StreamOptions {
	if !includeUsage {
		return nil
	}
	return &types.StreamOptions{IncludeUsage: true}
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvFloat32(key string, fallback float32) float32 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
