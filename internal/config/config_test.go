package config

import (
	"encoding/json"
	"testing"
)

// Legacy configs stored each profile as a bare token string. They must still
// load, and re-marshal into the new object form.
func TestLegacyProfileMigrates(t *testing.T) {
	raw := `{
		"server_url": "http://remote:3104",
		"current": "cuby",
		"profiles": {"cuby": "plane_api_legacy"}
	}`
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if got := c.ActiveToken(); got != "plane_api_legacy" {
		t.Fatalf("ActiveToken = %q, want plane_api_legacy", got)
	}
	// no per-profile server → falls back to the global default
	if got := c.ActiveServer(); got != "http://remote:3104" {
		t.Fatalf("ActiveServer = %q, want the global default", got)
	}

	// re-marshalling produces the object form
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Config
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if round.Profiles["cuby"].Token != "plane_api_legacy" {
		t.Fatalf("token lost across round-trip: %+v", round.Profiles["cuby"])
	}
}

// A profile pins its own server, so switching profiles switches the server too;
// UpsertProfile must not clobber a field when its argument is empty.
func TestPerProfileServer(t *testing.T) {
	var c Config
	c.ServerURL = "http://default:4006"

	c.UpsertProfile("ayetec", "tok_a", "http://localhost:4006")
	c.UpsertProfile("cuby", "tok_c", "http://remote:3104")

	c.Current = "ayetec"
	if c.ActiveToken() != "tok_a" || c.ActiveServer() != "http://localhost:4006" {
		t.Fatalf("ayetec active: token=%q server=%q", c.ActiveToken(), c.ActiveServer())
	}
	c.Current = "cuby"
	if c.ActiveToken() != "tok_c" || c.ActiveServer() != "http://remote:3104" {
		t.Fatalf("cuby active: token=%q server=%q", c.ActiveToken(), c.ActiveServer())
	}

	// updating only the server must preserve the token
	c.UpsertProfile("cuby", "", "http://remote2:3104")
	if c.ActiveToken() != "tok_c" {
		t.Fatalf("token clobbered by server-only upsert: %q", c.ActiveToken())
	}
	if c.ActiveServer() != "http://remote2:3104" {
		t.Fatalf("server not updated: %q", c.ActiveServer())
	}

	// a profile without its own server falls back to the global default
	c.UpsertProfile("bare", "tok_b", "")
	if c.ActiveServer() != "http://default:4006" {
		t.Fatalf("bare profile should fall back to default, got %q", c.ActiveServer())
	}
}
