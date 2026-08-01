// Package config loads and persists client/server settings. Everything lives in
// a single home directory (~/.bubble by default, or $BUBBLE_HOME): the client
// config.json and the server's bubble.db. Plane credentials live ONLY on the
// server side (§9.2).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Profile is one saved client target: a Plane credential paired with the server
// it should talk to. Binding the server to the profile means `bubble use <name>`
// switches BOTH the token and the server at once, so a client can't half-switch
// (e.g. an org's key pointed at the wrong server).
type Profile struct {
	Token  string `json:"token"`            // Plane API key (pass-through identity, §9.2)
	Server string `json:"server,omitempty"` // server base URL for this profile
}

// UnmarshalJSON accepts either the current object form ({"token","server"}) or
// the legacy bare-string form ("plane_api_…"), so older config files load and
// migrate transparently on the next Save.
func (p *Profile) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		p.Token = s
		return nil
	}
	type alias Profile // avoid recursion
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*p = Profile(a)
	return nil
}

// Config is the on-disk settings for both `bubble serve` and the thin client.
// Plane connections are NOT here — they live in the server's SQLite store as
// instances (managed with `bubble instance add`), since they are per-instance
// secrets the server owns (§9.2).
type Config struct {
	ServerURL string `json:"server_url"` // default client → server base URL (profile fallback)
	Addr      string `json:"addr"`       // server listen address

	CycleHours  int `json:"cycle_hours"`  // the heat-window pulse length (§1)
	TickMinutes int `json:"tick_minutes"` // server: how often to sweep for cooling bubbles (§5)

	AdminEmails []string `json:"admin_emails,omitempty"` // server: Plane emails granted godmode

	// Credential profiles let one client switch between workspaces/identities
	// (e.g. different Plane keys + servers per org). Current is the active one.
	Current  string             `json:"current,omitempty"`
	Profiles map[string]Profile `json:"profiles,omitempty"`

	Token string `json:"token,omitempty"` // legacy single credential (fallback)
}

// ActiveToken returns the credential for the active profile, falling back to the
// legacy single token if no profile is selected.
func (c Config) ActiveToken() string {
	if c.Current != "" {
		if p, ok := c.Profiles[c.Current]; ok {
			return p.Token
		}
	}
	return c.Token
}

// ActiveServer returns the server URL for the active profile, falling back to
// the global default when the profile pins no server of its own.
func (c Config) ActiveServer() string {
	if c.Current != "" {
		if p, ok := c.Profiles[c.Current]; ok && p.Server != "" {
			return p.Server
		}
	}
	return c.ServerURL
}

// UpsertProfile creates or updates a profile and makes it active. Empty token or
// server arguments leave the existing value untouched, so callers can set one
// field without clobbering the other.
func (c *Config) UpsertProfile(name, token, server string) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	p := c.Profiles[name]
	if token != "" {
		p.Token = token
	}
	if server != "" {
		p.Server = server
	}
	c.Profiles[name] = p
	c.Current = name
}

// ProfileNames returns the configured profile names, sorted.
func (c Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// dir is the single home for everything: $BUBBLE_HOME if set, else ~/.bubble.
func dir() (string, error) {
	if h := os.Getenv("BUBBLE_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bubble"), nil
}

// Home returns the single directory holding all Bubble Work state.
func Home() (string, error) {
	return dir()
}

// Path returns the config file location.
func Path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// DBPath returns the server's SQLite file location.
func DBPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "bubble.db"), nil
}

// Load reads the config, filling sensible defaults when the file is absent.
func Load() (Config, error) {
	c := Config{
		ServerURL:   "http://localhost:4006",
		Addr:        ":4006",
		CycleHours:  168, // one week
		TickMinutes: 60,  // sweep hourly for cooling bubbles
	}
	p, err := Path()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, nil
}

// Save persists the config, creating the directory if needed.
func Save(c Config) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	p := filepath.Join(d, "config.json")
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// Cycle returns the heat-window pulse as a duration.
func (c Config) Cycle() time.Duration {
	h := c.CycleHours
	if h <= 0 {
		h = 168
	}
	return time.Duration(h) * time.Hour
}

// TickInterval returns how often the server sweeps for cooling bubbles.
// Zero disables the automatic ticker.
func (c Config) TickInterval() time.Duration {
	return time.Duration(c.TickMinutes) * time.Minute
}
