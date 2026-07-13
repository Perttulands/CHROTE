package main

import "testing"

func TestPanicRecoveryMiddlewareFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.2 after server handlers are wrapped with panic recovery that returns a clean 500")
}
