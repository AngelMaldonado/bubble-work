// Package config loads and persists client/server settings from the XDG config
// dir. The Plane credentials live ONLY on the server side (§9.2).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config is the on-disk settings for both `bubble serve` and the thin client.
type Config struct {
	ServerURL string `json:"server_url"` // client → server base URL
	Addr      string `json:"addr"`       // server listen address

	// Server-only: Plane connection (the server is Plane's sole client, §9.6).
	PlaneBaseURL   string `json:"plane_base_url"`
	PlaneAPIKey    string `json:"plane_api_key"`
	PlaneWorkspace string `json:"plane_workspace"` // Plane workspace slug
	PlaneProject   string `json:"plane_project"`   // Plane project uuid = our Workspace (§7.1)

	CycleHours int    `json:"cycle_hours"` // the heat-window pulse length (§1)
	Token      string `json:"token"`       // this member's credential to the server
}

func dir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "bubble"), nil
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
		ServerURL:    "http://localhost:4006",
		Addr:         ":4006",
		PlaneBaseURL: "https://api.plane.so",
		CycleHours:   168, // one week
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
