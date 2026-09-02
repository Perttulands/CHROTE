package proxy

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// originPolicy decides which browser origins may open a terminal WebSocket.
//
// CHROTE has no application login and a terminal is arbitrary command execution
// as its Unix user, so the one thing standing between a page the operator did
// not open and a reachable CHROTE is this check. CORS response headers cannot
// do it: a WebSocket handshake is not subject to the same-origin policy, so the
// browser hands the socket to the page whatever headers come back. The upgrade
// therefore applies the configured policy itself, rather than trusting the CORS
// middleware wrapped around the same mux to have refused anything.
//
// This is not authentication. It stops a foreign page in the operator's browser
// from reaching a CHROTE that page can route to; it identifies nobody and stops
// no direct network client. The private-network perimeter remains the boundary.
type originPolicy struct {
	// configured mirrors CORS_ORIGINS, matched exactly the way the CORS
	// middleware matches it, so one setting means one thing everywhere.
	configured map[string]struct{}
}

func newOriginPolicy(configured []string) originPolicy {
	allowed := make(map[string]struct{}, len(configured))
	for _, origin := range configured {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return originPolicy{configured: allowed}
}

// allows reports whether a handshake may proceed.
//
// An absent Origin is not a foreign one. Browsers always send it on a WebSocket
// handshake; non-browser clients — curl, a script, a health probe — send none,
// and those are governed by what CHROTE is bound to, which is the documented
// boundary. Refusing them would break local tooling and protect nothing.
//
// With no configured origins the policy is same-origin: the dashboard is served
// by this same server, so it is the only page that legitimately opens this
// socket. Preserving today's accept-everything default would leave the hole the
// whole check exists to close, and a deployment that genuinely serves the
// dashboard from elsewhere already says so through CORS_ORIGINS.
func (p originPolicy) allows(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if _, ok := p.configured[origin]; ok {
		return true
	}
	return sameOrigin(origin, r.Host)
}

// sameOrigin compares an Origin against the host the request was addressed to,
// on host and port only.
//
// The scheme is deliberately not compared. A private access layer commonly
// terminates TLS and forwards plain HTTP to a loopback bind, so the browser's
// origin is https while the request CHROTE sees is not; requiring them to agree
// would reject the operator's own remote dashboard. Tailscale's proxy, the
// documented access layer, forwards the original Host unchanged, which is what
// makes the comparison work at all. A same-host attacker on the other scheme is
// already inside the perimeter this check does not replace.
func sameOrigin(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	// An opaque Origin — `null`, sent by a sandboxed frame or a file:// page —
	// parses with no host and is refused here.
	if err != nil || parsed.Host == "" {
		return false
	}
	return normalizeHost(parsed.Host) == normalizeHost(requestHost)
}

// normalizeHost lowercases a host and drops a port only the URL scheme's
// default could have put there, since a browser omits those from an Origin
// while a Host header may carry them.
func normalizeHost(hostPort string) string {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return strings.ToLower(hostPort)
	}
	if port == "80" || port == "443" {
		return strings.ToLower(host)
	}
	return strings.ToLower(net.JoinHostPort(host, port))
}
