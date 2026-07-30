package config

import (
	"os"
)

type Config struct {
	Addr             string  //服务器监听地址
	DeepSeekKey      string  //DeepSeek API Key
	ModelName        string  //模型名
	MaxTokens        int     //最大token数
	Temperature      float32 //模型温度（影响随机）
	Thinking         string  //enabled/disabled
	Reasoning_Effort string  //high/max
}

func Load() *Config {
	return &Config{
		Addr:             getEnv("ADDR", ":8080"),
		DeepSeekKey:      getEnv("DEEPSEEK_API_KEY", ""),
		ModelName:        getEnv("MODEL_NAME", "deepseek-v4-flash"),
		MaxTokens:        4096,
		Temperature:      0.7,
		Thinking:         "enabled",
		Reasoning_Effort: "high",
	}
}

func getEnv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
