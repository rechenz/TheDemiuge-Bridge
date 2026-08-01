package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/registry"
)

func Quit(h *server.Hertz) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	<-quit
	log.Println("正在关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.Shutdown(ctx)
}

func main() {
	cfg := config.Load()

	// 从 YAML 文件加载 Agent / Tool 注册(存储位置见 config.Registry)。
	// 先加载全局工具表,再按引用名称把工具绑定到各 Agent 的可调用范围。
	toolRegistry, err := registry.LoadTools(cfg.Registry.ToolsFile)
	if err != nil {
		log.Fatalf("加载 Tool 注册失败: %v", err)
	}
	agentRegistry, err := registry.LoadAgents(cfg.Registry.AgentsFile, toolRegistry)
	if err != nil {
		log.Fatalf("加载 Agent 注册失败: %v", err)
	}

	log.Printf("已从 %s 注册 %d 个 Tool", cfg.Registry.ToolsFile, len(toolRegistry.All()))
	log.Printf("已从 %s 注册 %d 个 Agent", cfg.Registry.AgentsFile, len(agentRegistry.All()))

	h := server.Default(
		server.WithHostPorts(cfg.Addr),
		server.WithExitWaitTime(5*time.Second),
	)

	go Quit(h)

	h.Spin()
}
