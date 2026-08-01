package mcp

import (
	"sync"
	"testing"
	"time"
)

// TestHub_BroadcastCancelRace 验证 Broadcast 与 cancel 并发执行时不会发生
// "send on closed channel" panic(修复前 Broadcast 在锁外发送,可能向已 close 的通道发送)。
func TestHub_BroadcastCancelRace(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe("inst_a")

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 并发广播者:持续向 inst_a 广播,直到测试结束
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				hub.Broadcast("inst_a", Notification{JSONRPC: RPCVersion, Method: MethodNotifToolsChanged})
			}
		}
	}()

	// 主线程稍作等待,确保广播循环启动,然后取消订阅(close 通道)
	time.Sleep(2 * time.Millisecond)
	cancel()

	// 让广播循环再跑一段时间,触发发送与 close 的竞态窗口
	time.Sleep(5 * time.Millisecond)
	close(stop)
	wg.Wait()

	// 订阅者通道应已关闭:持续读取,排空关闭前可能写入的缓冲通知,
	// 最终读取到 (零值, false) 即证明通道已 close(且不会 panic)。
	deadline := time.Now().Add(time.Second)
	for {
		_, ok := <-ch
		if !ok {
			break // 通道已关闭且排空
		}
		if time.Now().After(deadline) {
			t.Fatal("取消订阅后通道迟迟未关闭")
		}
	}

	// 取消后订阅集合应清空
	if n := hub.SubscriberCount("inst_a"); n != 0 {
		t.Errorf("取消后订阅者数 = %d,期望 0", n)
	}
}

// TestHub_SubscribeBroadcast 验证正常订阅-广播-接收闭环。
func TestHub_SubscribeBroadcast(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe("inst_a")
	defer cancel()

	hub.Broadcast("inst_a", Notification{JSONRPC: RPCVersion, Method: MethodNotifToolsChanged})

	select {
	case n := <-ch:
		if n.Method != MethodNotifToolsChanged {
			t.Errorf("通知 method = %q,期望 %q", n.Method, MethodNotifToolsChanged)
		}
	case <-time.After(time.Second):
		t.Fatal("等待通知超时")
	}
}

// TestHub_MultiSubscribers 验证同一实例的多个订阅者都能收到通知。
func TestHub_MultiSubscribers(t *testing.T) {
	hub := NewHub()
	ch1, c1 := hub.Subscribe("inst_a")
	defer c1()
	ch2, c2 := hub.Subscribe("inst_a")
	defer c2()

	hub.Broadcast("inst_a", Notification{JSONRPC: RPCVersion, Method: MethodNotifAgentsChanged})

	for _, ch := range []<-chan Notification{ch1, ch2} {
		select {
		case n := <-ch:
			if n.Method != MethodNotifAgentsChanged {
				t.Errorf("通知 method = %q", n.Method)
			}
		case <-time.After(time.Second):
			t.Fatal("等待通知超时")
		}
	}
}

// TestHub_BroadcastDropsWhenFull 验证通道满时广播丢弃而非阻塞。
func TestHub_BroadcastDropsWhenFull(t *testing.T) {
	hub := NewHub()
	ch, cancel := hub.Subscribe("inst_a")
	defer cancel()

	// 填满 16 缓冲
	for i := 0; i < 16; i++ {
		hub.Broadcast("inst_a", Notification{JSONRPC: RPCVersion, Method: MethodNotifToolsChanged})
	}

	// 第 17 条应被丢弃(不阻塞)
	done := make(chan struct{})
	go func() {
		hub.Broadcast("inst_a", Notification{JSONRPC: RPCVersion, Method: MethodNotifToolsChanged})
		close(done)
	}()
	select {
	case <-done:
		// OK:未阻塞
	case <-time.After(time.Second):
		t.Fatal("通道满时 Broadcast 不应阻塞")
	}

	// 清空后可再接收,证明缓冲仍工作
	for i := 0; i < 16; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("第 %d 条缓冲通知丢失", i+1)
		}
	}
}
