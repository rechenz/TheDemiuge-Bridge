package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/llm"
	"github.com/rechenz/TheDemiuge-Bridge/internal/mcp"
	"github.com/rechenz/TheDemiuge-Bridge/internal/server/handler"
	"github.com/rechenz/TheDemiuge-Bridge/internal/backend"
)

func main() {
	cfg := config.Load()

	// ── UE5 实例注册中心 + 变更广播 ─────────────────────────────────────────
	hub := mcp.NewHub()
	mgr := backend.NewManager(
		backend.WithRegistryDir(cfg.UE5.RegistryDir),
		backend.WithChangeListener(hub.OnChange),
	)
	if err := mgr.Restore(); err != nil {
		log.Fatalf("恢复 UE5 实例注册失败: %v", err)
	}
	instances := mgr.InstanceIDs()
	if len(instances) > 0 {
		hlog.Infof("已从 %s 恢复 %d 个 UE5 实例", cfg.UE5.RegistryDir, len(instances))
	}

	// ── UE5 工具执行转发客户端 ──────────────────────────────────────────────
	cli := &backend.Client{
		DefaultEndpoint: cfg.UE5.DefaultEndpoint,
		HTTPTimeout:     cfg.HTTPTimeout,
		HTTPClient:      cfg.HTTPClient,
	}

	// ── 处理器 ───────────────────────────────────────────────────────────────
	// backend.RegistryAdapter 把"UE5 注册中心 + HTTP 转发"实现为通用 mcp.Registry,
	// 注入 MCP Server 与 ChatHandler——协议层不依赖任何特定后端。
	ue5Handler := handler.NewUE5Handler(mgr, &cfg.UE5)
	adapter := backend.NewRegistryAdapter(mgr, cli)
	mcpServer := mcp.NewServer(adapter)
	mcpHandler := handler.NewMCPHandler(mcpServer, hub)
	chatHandler := handler.NewChatHandler(adapter, llm.NewDeepseekProvider(cfg), false)

	// ── Hertz 服务器 ─────────────────────────────────────────────────────────
	h := server.Default(
		server.WithHostPorts(cfg.Addr),
		server.WithExitWaitTime(5*time.Second),
		server.WithTransport(standard.NewTransporter),
	)

	// MCP 入口(未来 agent 通过 /mcp/{instance_id} 访问该 UE5 实例的能力)
	mcpGroup := h.Group("/mcp")
	mcpHandler.RegisterRoutes(mcpGroup)

	// NPC 对话入口(Bridge 自身跑 ReAct:玩家 → DeepSeek → 工具 → UE5)
	chatGroup := h.Group("/api/chat")
	chatGroup.Use(chatHandler.AuthMiddleware(cfg))
	chatGroup.POST("", chatHandler.Chat)

	// UE5 管理接口(UE5 插件通过 /api/v1/ue5/* 动态注册 agent/tool)
	ue5Group := h.Group("/api/v1/ue5")
	ue5Group.Use(ue5Handler.AuthMiddleware())
	ue5Group.Use(logMiddleware())
	ue5Handler.RegisterRoutes(ue5Group)

	// 健康检查
	h.GET("/api/v1/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(consts.StatusOK, map[string]any{
			"status":    "ok",
			"instances": len(mgr.InstanceIDs()),
		})
	})

	// 优雅退出
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		<-quit
		hlog.Info("正在关闭服务器...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
	}()

	hlog.Infof("TheDemiuge-Bridge 启动: 监听 %s", cfg.Addr)
	h.Spin()
}

// logMiddleware 简单的访问日志(UE5 管理接口)。
func logMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Next(ctx)
		hlog.Infof("[UE5-API] %s %s -> %d", c.Request.Method(), c.Request.Path(), c.Response.StatusCode())
	}
}
