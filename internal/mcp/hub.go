package mcp

import (
	"sync"

	"github.com/rechenz/TheDemiuge-Bridge/internal/ue5"
)

// Hub 管理 SSE 长连接的订阅者,按实例隔离。
// UE5 实例的 agent/tool 注册变更时,通过 ChangeListener 回调
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
func (h *Hub) Broadcast(instanceID string, n Notification) {
	h.mu.Lock()
	set := h.subs[instanceID]
	// 复制订阅者集合,释放锁后再逐个推送
	subs := make([]chan Notification, 0, len(set))
	for ch := range set {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- n:
		default:
			// 通道满,丢弃该通知
		}
	}
}

// OnChange 实现 ue5.ChangeListener。
// 由 manager 在注册变更时同步调用,转换为 MCP 通知广播。
func (h *Hub) OnChange(c ue5.Change) {
	switch c.Kind {
	case ue5.ChangeTool:
		h.Broadcast(c.InstanceID, Notification{
			JSONRPC: RPCVersion,
			Method:  MethodNotifToolsChanged,
		})
	case ue5.ChangeAgent:
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
