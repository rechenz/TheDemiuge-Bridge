// Package handler 提供 HTTP 业务处理器。
package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"

	"github.com/rechenz/TheDemiuge-Bridge/internal/config"
	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// UE5APIKeyHeader UE5 管理接口鉴权 header。
const UE5APIKeyHeader = "X-UE5-Key"

// UE5Handler 处理 UE5 实例的管理接口。
// 所有注册、查询、注销操作委托给 ue5.Manager;
// 工具执行转发由 ue5.Client 完成(MCP 层共用)。
type UE5Handler struct {
	mgr *ue5.Manager
	cfg *config.UE5Config
}

// NewUE5Handler 构造 UE5 管理处理器。
func NewUE5Handler(mgr *ue5.Manager, cfg *config.UE5Config) *UE5Handler {
	return &UE5Handler{mgr: mgr, cfg: cfg}
}

// AuthMiddleware 校验 X-UE5-Key。
// 配置为空时不鉴权(本地联调);校验失败返回 401。
func (h *UE5Handler) AuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if h.cfg != nil && h.cfg.APIKey != "" {
			if c.Request.Header.Get(UE5APIKeyHeader) != h.cfg.APIKey {
				c.JSON(consts.StatusUnauthorized, map[string]any{
					"error": "X-UE5-Key 不匹配",
				})
				c.Abort()
				return
			}
		}
		c.Next(ctx)
	}
}

// RegisterRoutes 注册 UE5 管理路由。
// 挂载在 /api/v1/ue5 下(由调用方传入路由组或直接挂 Hertz)。
// 注意:auth 中间件需要在调用方侧对 /api/v1/ue5/* 统一挂载。
func (h *UE5Handler) RegisterRoutes(group *route.RouterGroup) {
	// 实例管理
	group.POST("/instances", h.createInstance)
	group.GET("/instances", h.listInstances)
	group.GET("/instances/:id", h.getInstance)
	group.DELETE("/instances/:id", h.deleteInstance)

	// agent 管理
	group.POST("/instances/:id/agents", h.batchUpsertAgents)
	group.GET("/instances/:id/agents", h.listAgents)
	group.POST("/instances/:id/agents/:name", h.upsertAgent)
	group.GET("/instances/:id/agents/:name", h.getAgent)
	group.DELETE("/instances/:id/agents/:name", h.deleteAgent)

	// tool 管理
	group.POST("/instances/:id/tools", h.batchUpsertTools)
	group.GET("/instances/:id/tools", h.listTools)
	group.POST("/instances/:id/tools/:name", h.upsertTool)
	group.GET("/instances/:id/tools/:name", h.getTool)
	group.DELETE("/instances/:id/tools/:name", h.deleteTool)
}

// ── 实例管理 ────────────────────────────────────────────────────────────────

// createInstance 创建或更新实例。
// body: {"id": "instance_1", "default_endpoint": "http://127.0.0.1:9000"}
func (h *UE5Handler) createInstance(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ID              string `json:"id"`
		DefaultEndpoint string `json:"default_endpoint"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "请求体非法: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "id 不能为空"})
		return
	}
	h.mgr.RegisterInstance(req.ID, req.DefaultEndpoint)
	c.JSON(consts.StatusOK, ue5.InstanceInfo{
		ID:              req.ID,
		DefaultEndpoint: req.DefaultEndpoint,
	})
}

// listInstances 返回全部实例概要。
func (h *UE5Handler) listInstances(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, h.mgr.InstancesInfo())
}

// getInstance 查询单个实例概要。
func (h *UE5Handler) getInstance(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	inst, ok := h.mgr.GetInstance(id)
	if !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "实例不存在"})
		return
	}
	c.JSON(consts.StatusOK, inst.Info())
}

// deleteInstance 注销实例。
func (h *UE5Handler) deleteInstance(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if ok := h.mgr.RemoveInstance(id); !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "实例不存在"})
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"deleted": id})
}

// ── Agent 管理 ──────────────────────────────────────────────────────────────

// batchUpsertAgents 批量注册 agent(agents.yaml 清单上传)。
// body: {"agents": [AgentDef...]}
func (h *UE5Handler) batchUpsertAgents(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Agents []ue5.AgentDef `json:"agents"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "请求体非法: " + err.Error()})
		return
	}
	if len(req.Agents) == 0 {
		c.JSON(consts.StatusOK, map[string]any{"registered": 0})
		return
	}
	if err := h.mgr.UpsertAgents(c.Param("id"), req.Agents); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"registered": len(req.Agents)})
}

// upsertAgent 注册/更新单个 agent(agent_name.yaml)。
func (h *UE5Handler) upsertAgent(ctx context.Context, c *app.RequestContext) {
	name := c.Param("name")
	var def ue5.AgentDef
	if err := c.BindJSON(&def); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "请求体非法: " + err.Error()})
		return
	}
	def.Name = name
	if err := h.mgr.UpsertAgent(c.Param("id"), def); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, def)
}

// getAgent 查询单个 agent。
func (h *UE5Handler) getAgent(ctx context.Context, c *app.RequestContext) {
	def, ok := h.mgr.GetAgent(c.Param("id"), c.Param("name"))
	if !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "agent 不存在"})
		return
	}
	c.JSON(consts.StatusOK, def)
}

// listAgents 列出实例全部 agent。
func (h *UE5Handler) listAgents(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, h.mgr.Agents(c.Param("id")))
}

// deleteAgent 删除 agent。
func (h *UE5Handler) deleteAgent(ctx context.Context, c *app.RequestContext) {
	id, name := c.Param("id"), c.Param("name")
	if ok := h.mgr.DeleteAgent(id, name); !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "agent 不存在"})
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"deleted": name})
}

// ── Tool 管理 ───────────────────────────────────────────────────────────────

// batchUpsertTools 批量注册 tool(tools.yaml 清单上传)。
// body: {"tools": [ToolReg...]}(ToolReg 内嵌 ToolDef + endpoint)
func (h *UE5Handler) batchUpsertTools(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Tools []ue5.ToolReg `json:"tools"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "请求体非法: " + err.Error()})
		return
	}
	if len(req.Tools) == 0 {
		c.JSON(consts.StatusOK, map[string]any{"registered": 0})
		return
	}
	if err := h.mgr.UpsertTools(c.Param("id"), req.Tools); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"registered": len(req.Tools)})
}

// upsertTool 注册/更新单个 tool(定义 + 转发 endpoint)。
func (h *UE5Handler) upsertTool(ctx context.Context, c *app.RequestContext) {
	name := c.Param("name")
	var reg ue5.ToolReg
	if err := c.BindJSON(&reg); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": "请求体非法: " + err.Error()})
		return
	}
	reg.Name = name
	if err := h.mgr.UpsertTool(c.Param("id"), reg); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, reg)
}

// getTool 查询单个 tool。
func (h *UE5Handler) getTool(ctx context.Context, c *app.RequestContext) {
	reg, ok := h.mgr.GetTool(c.Param("id"), c.Param("name"))
	if !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "tool 不存在"})
		return
	}
	c.JSON(consts.StatusOK, reg)
}

// listTools 列出实例全部 tool。
func (h *UE5Handler) listTools(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, h.mgr.Tools(c.Param("id")))
}

// deleteTool 删除 tool。
func (h *UE5Handler) deleteTool(ctx context.Context, c *app.RequestContext) {
	id, name := c.Param("id"), c.Param("name")
	if ok := h.mgr.DeleteTool(id, name); !ok {
		c.JSON(consts.StatusNotFound, map[string]any{"error": "tool 不存在"})
		return
	}
	c.JSON(consts.StatusOK, map[string]any{"deleted": name})
}

// httpStatusOK 便于引用 http 包(防止误删)。
var _ = http.StatusOK
