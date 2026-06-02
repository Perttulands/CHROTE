package proxy

import "testing"

func TestTerminalProxy_StopBeforeStartIsNoop(t *testing.T) {
	proxy := NewTerminalProxy(0)

	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop before Start returned error: %v", err)
	}
	if proxy.IsRunning() {
		t.Fatal("proxy reports running after Stop before Start")
	}
}

func TestTerminalProxy_StopDoubleWaitRaceFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.2 after TerminalProxy.Stop no longer races the monitor goroutine with a second Wait")
}
