package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// DNS-rebinding protection for the MCP endpoint.
//
// MCP's own rule is: a request arriving over loopback whose Host header is not
// loopback is refused. The threat is real — a malicious page can make a
// browser POST to http://localhost:<port>/mcp, and without the check a local
// MCP server would happily serve it.
//
// Behind a reverse proxy every legitimate request looks exactly like that.
// colima forwards ports over an SSH tunnel, so requests reach the server on
// 127.0.0.1 carrying the real public Host, and MCP refused all of them with
// "Forbidden: invalid Host header". The web UI kept working because the rule
// lives only in the MCP handler, which is why the board was fine and only
// agents could not connect.
//
// So the SDK's blanket rule is turned off and this takes its place: the same
// protection, with an explicit allowlist. Not a weakening — an empty list
// behaves exactly as the SDK did.

// SetMCPHosts records the Host headers /mcp will answer to besides loopback.
func (s *Server) SetMCPHosts(hosts []string) {
	s.mcpHosts = make(map[string]bool, len(hosts))
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			s.mcpHosts[hostOnly(h)] = true
		}
	}
}

// mcpHostGuard refuses a request whose Host is neither loopback nor allowlisted.
//
// It only applies when the request actually arrived over loopback, exactly like
// the rule it replaces: a server reachable directly on its own interface is not
// the case DNS rebinding attacks.
func (s *Server) mcpHostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.mcpHostAllowed(r) {
			next.ServeHTTP(w, r)
			return
		}
		// Say what to do about it. The SDK's message names the rejected host and
		// stops there, which sends people hunting through proxy config for a
		// problem that is in this server's configuration.
		http.Error(w, fmt.Sprintf(
			"Forbidden: this server does not answer MCP for Host %q. "+
				"Add it to mcp_hosts in the server config if that is the name it is served under.",
			r.Host), http.StatusForbidden)
	})
}

func (s *Server) mcpHostAllowed(r *http.Request) bool {
	host := hostOnly(strings.ToLower(r.Host))
	if s.mcpHosts[host] {
		return true
	}
	// Loopback Host is always fine — that is a client talking to a server on the
	// same machine, which is what the protection is FOR rather than against.
	if isLoopbackHost(host) {
		return true
	}
	// A request that did not arrive over loopback was never in scope: if it
	// reached this interface it did not come through a victim's browser
	// pointing at localhost.
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || local == nil {
		return false
	}
	lh, _, err := net.SplitHostPort(local.String())
	if err != nil {
		lh = local.String()
	}
	return !isLoopbackHost(lh)
}

// hostOnly strips a port, tolerating IPv6 literals.
func hostOnly(h string) string {
	if v, _, err := net.SplitHostPort(h); err == nil {
		return v
	}
	return strings.Trim(h, "[]")
}

func isLoopbackHost(h string) bool {
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
