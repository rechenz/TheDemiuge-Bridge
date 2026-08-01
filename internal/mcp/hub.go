package mcp

import (
	"sync"
)

// Hub 管理 SSE 长连接的订阅者,按实例隔离。
// 注册中心(如 UE5)发生 agent/tool 变更时,通过 OnChange 回调
// 向对应实例的所有订阅者广播 list_changed 通知。
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Notification]struct{} // instanceID -> 订阅者通道
}

// NewHub 构造空的广播中心。
func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Notification]struct{})}
}

// Subscribe 订阅某实例的变更通知。
// 返回接收通道与取消订阅函数(调用方在连接关闭时必须调用)。
func (h *Hub) Subscribe(instanceID string) (<-chan Notification, func()) {
	ch := make(chan Notification, 16)

	h.mu.Lock()
	defer h.mu.Unlock()

	set := h.subs[instanceID]
	if set == nil {
		set = make(map[chan Notification]struct{})
		h.subs[instanceID] = set
	}
	set[ch] = struct{}{}

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if set, ok := h.subs[instanceID]; ok {
			if _, ok := set[ch]; ok {
				delete(set, ch)
				if len(set) == 0 {
					delete(h.subs, instanceID)
				}
			}
		}
		close(ch)
	}
	return ch, cancel
}

// Broadcast 向某实例的全部订阅者推送通知。
// 发送不阻塞:订阅者通道满时丢弃(避免慢消费者拖垮注册流程)。
// 发送在持有锁的情况下进行,与 Subscribe 的 close 互斥,
// 避免"发送到已关闭通道"导致的 panic(cancel 在锁内 close(ch))。
func (h *Hub) Broadcast(instanceID string, n Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[instanceID] {
		select {
		case ch <- n:
		default:
			// 通道满,丢弃该通知
		}
	}
}

// OnChange 实现注册变更回调。
// 由注册中心(Manager)在 tool/agent 变更时同步调用,转换为 MCP 通知广播。
// 回调需立即返回,不得阻塞。
func (h *Hub) OnChange(c Change) {
	switch c.Kind {
	case ChangeTool:
		h.Broadcast(c.InstanceID, Notification{
			JSONRPC: RPCVersion,
			Method:  MethodNotifToolsChanged,
		})
	case ChangeAgent:
		h.Broadcast(c.InstanceID, Notification{
			JSONRPC: RPCVersion,
			Method:  MethodNotifAgentsChanged,
		})
	}
}

// SubscriberCount 返回指定实例的订阅者数量(测试用)。
func (h *Hub) SubscriberCount(instanceID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[instanceID])
}
