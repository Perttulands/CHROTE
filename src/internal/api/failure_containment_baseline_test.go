package api

import "testing"

func TestOracleHandler_StopIsIdempotent(t *testing.T) {
	handler := NewOracleHandler(NewTmuxHandler(), NewBeadsHandler())

	handler.Stop()
	handler.Stop()
}

func TestOracleHandler_RestartAfterStopFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.2 after Oracle poller lifecycle can restart without closing an already-closed channel")
}

func TestOracleHandler_PollerPanicRecoveryFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.2 after poller panics are recovered and reported without killing the goroutine")
}
