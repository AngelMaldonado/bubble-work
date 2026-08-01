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

// Config is the on-disk settings for both `bubble serve` and the thin client.
// Plane connections are NOT here — they live in the server's SQLite store as
// instances (managed with `bubble instance add`), since they are per-instance
// secrets the server owns (§9.2).
type Config struct {
	ServerURL string `json:"server_url"` // client → server base URL
	Addr      string `json:"addr"`       // server listen address

	CycleHours  int `json:"cycle_hours"`  // the heat-window pulse length (§1)
	TickMinutes int `json:"tick_minutes"` // server: how often to sweep for cooling bubbles (§5)

	AdminEmails []string `json:"admin_emails,omitempty"` // server: Plane emails granted godmode

	// Credential profiles let one client switch between workspaces/identities
	// (e.g. different Plane keys per org). Current is the active profile name.
	Current  string            `json:"current,omitempty"`
	Profiles map[string]string `json:"profiles,omitempty"` // name -> Plane API key

	Token string `json:"token,omitempty"` // legacy single credential (fallback)
}

// ActiveToken returns the credential for the active profile, falling back to the
// legacy single token if no profile is selected.
func (c Config) ActiveToken() string {
	if c.Current != "" {
		if t, ok := c.Profiles[c.Current]; ok {
			return t
		}
	}
	return c.Token
}

// SetProfile stores a credential under name and makes it the active profile.
func (c *Config) SetProfile(name, token string) {
	if c.Profiles == nil {
		c.Profiles = map[string]string{}
	}
	c.Profiles[name] = token
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
