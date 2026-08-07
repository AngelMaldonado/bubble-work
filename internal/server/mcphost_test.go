package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bug this exists for: colima forwards ports over an SSH tunnel, so a
// request that came through the reverse proxy reaches the server on 127.0.0.1
// carrying the real public Host — which is indistinguishable, to MCP's
// DNS-rebinding rule, from a malicious page driving a local server through a
// victim's browser. Every agent got "Forbidden: invalid Host header".
func TestMCPHostGuard(t *testing.T) {
	srv := &Server{mcpHosts: map[string]bool{}}
	srv.SetMCPHosts([]string{"bubble.cubytest.space", "  Bubble.Angel.CubyTest.Space  "})

	call := func(host string, localAddr string) int {
		t.Helper()
		reached := false
		h := srv.mcpHostGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.Host = host
		if localAddr != "" {
			addr, _ := net.ResolveTCPAddr("tcp", localAddr)
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, addr))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if reached && w.Code != http.StatusOK {
			t.Fatal("handler ran but a status was written")
		}
		return w.Code
	}

	const loopback = "127.0.0.1:3104"

	// The reported failure: proxied in, arrives on loopback, real public Host.
	if code := call("bubble.cubytest.space", loopback); code != http.StatusOK {
		t.Errorf("an allowlisted host was refused: %d", code)
	}
	// Case and stray whitespace in config must not decide who gets in.
	if code := call("bubble.angel.cubytest.space", loopback); code != http.StatusOK {
		t.Errorf("allowlist is case-sensitive: %d", code)
	}
	// A port on the Host header is not part of the name.
	if code := call("bubble.cubytest.space:443", loopback); code != http.StatusOK {
		t.Errorf("a port on the Host defeated the allowlist: %d", code)
	}

	// The protection itself still works: this is the actual rebinding shape —
	// loopback connection, some attacker-chosen name that we never allowed.
	if code := call("evil.example.com", loopback); code != http.StatusForbidden {
		t.Errorf("DNS rebinding was allowed through: %d", code)
	}

	// A client on the same machine is what the rule protects, not what it
	// blocks, so loopback Host is always fine.
	for _, h := range []string{"localhost", "127.0.0.1:3104", "[::1]:3104"} {
		if code := call(h, loopback); code != http.StatusOK {
			t.Errorf("loopback Host %q was refused: %d", h, code)
		}
	}

	// A request that did NOT arrive over loopback was never in scope: it did
	// not come through a browser pointed at localhost.
	if code := call("anything.example.com", "192.168.20.102:3104"); code != http.StatusOK {
		t.Errorf("a non-loopback request was judged by the rebinding rule: %d", code)
	}

	// And with nothing configured, behaviour is exactly the SDK's: loopback only.
	bare := &Server{mcpHosts: map[string]bool{}}
	h := bare.mcpHostGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Host = "bubble.cubytest.space"
	addr, _ := net.ResolveTCPAddr("tcp", loopback)
	r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, addr))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("an empty allowlist should be loopback-only, got %d", w.Code)
	}
	if body := w.Body.String(); !contains(body, "mcp_hosts") {
		t.Errorf("the refusal does not say how to fix it: %s", body)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
