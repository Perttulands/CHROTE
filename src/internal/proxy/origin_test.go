package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The handler-level Origin cases live in terminal_test.go. These cover the
// comparison itself, where the awkward shapes are: a browser omits a default
// port from an Origin that a Host header may still carry, a TLS-terminating
// access layer forwards plain HTTP, and an opaque origin has no host at all.
func TestOriginPolicy_Allows(t *testing.T) {
	tests := []struct {
		name       string
		configured []string
		origin     string
		host       string
		want       bool
	}{
		{name: "no Origin at all", host: "chrote.example:8094", want: true},
		{name: "same origin", origin: "http://chrote.example:8094", host: "chrote.example:8094", want: true},
		{name: "foreign origin", origin: "https://evil.example", host: "chrote.example:8094"},
		{name: "same host, different port", origin: "http://chrote.example:9999", host: "chrote.example:8094"},
		{
			name:   "TLS terminated in front of a plain-HTTP bind",
			origin: "https://chrote.example:8445", host: "chrote.example:8445", want: true,
		},
		{name: "default port omitted from the Origin", origin: "https://chrote.example", host: "chrote.example:443", want: true},
		{name: "host casing", origin: "http://CHROTE.example:8094", host: "chrote.example:8094", want: true},
		{name: "loopback", origin: "http://127.0.0.1:8094", host: "127.0.0.1:8094", want: true},
		{name: "IPv6 loopback", origin: "http://[::1]:8094", host: "[::1]:8094", want: true},
		{name: "opaque origin from a sandboxed frame", origin: "null", host: "chrote.example:8094"},
		{name: "configured origin", configured: []string{"https://desk.example"}, origin: "https://desk.example", host: "chrote.example:8094", want: true},
		{name: "origin outside the configured list", configured: []string{"https://desk.example"}, origin: "https://evil.example", host: "chrote.example:8094"},
		{name: "empty configured entries are not an allowed origin", configured: []string{"", "  "}, origin: "https://evil.example", host: "chrote.example:8094"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/terminal/ws", nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}

			if got := newOriginPolicy(test.configured).allows(request); got != test.want {
				t.Fatalf("allows(origin=%q, host=%q) = %v, want %v", test.origin, test.host, got, test.want)
			}
		})
	}
}
