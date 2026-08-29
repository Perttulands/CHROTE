package proxy

import "testing"

func TestTerminalProxy_StopBeforeStartIsNoop(t *testing.T) {
	proxy := NewTerminalProxy(0)

	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop before Start returned error: %v", err)
	}
}
