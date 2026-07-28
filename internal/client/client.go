// Package client is the thin human-facing CLI client. It talks ONLY to the
// Bubble Work server (never to Plane) and renders the buoyancy view.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

func getJSON(url, token string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized — set your member token with `bubble init --token <token>` " +
			"(create one on the server host with `bubble member add`)")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func icon(l domain.Lifecycle) string {
	switch l {
	case domain.Hot:
		return "🔥"
	case domain.Warm:
		return "☀️"
	case domain.Cooling:
		return "💧"
	case domain.Dormant:
		return "🧊"
	case domain.Closed:
		return "⚫"
	default:
		return "•"
	}
}

// Ls renders bubbles hottest-first — the "tend what floats at the top" view.
func Ls(cfg config.Config) error {
	var vs []domain.BubbleView
	if err := getJSON(cfg.ServerURL+"/api/bubbles", cfg.ActiveToken(), &vs); err != nil {
		return err
	}
	if len(vs) == 0 {
		fmt.Println("No bubbles here. Register a Plane instance (`bubble instance add`) you're a member of, then create a Module in Plane. (Check `bubble whoami` for your instances.)")
		return nil
	}
	for _, v := range vs {
		fmt.Printf("%s  %-10s  %-26s  %-8s  %s\n", icon(v.Lifecycle), v.Instance, v.Name, v.Lifecycle, v.Reason)
	}
	return nil
}

// Whoami shows the identity the server resolved from your credential — proof
// that Plane pass-through auth worked, and which instances/role Plane granted.
func Whoami(cfg config.Config) error {
	var a domain.Actor
	if err := getJSON(cfg.ServerURL+"/api/whoami", cfg.ActiveToken(), &a); err != nil {
		return err
	}
	fmt.Printf("name      : %s\n", a.Name)
	fmt.Printf("kind      : %s\n", a.Kind)
	if a.Email != "" {
		fmt.Printf("email     : %s\n", a.Email)
	}
	fmt.Printf("admin     : %v\n", a.Admin)
	if len(a.Instances) == 0 {
		fmt.Println("instances : (none — no Plane membership matched, or no grants)")
	} else {
		fmt.Printf("instances : %s\n", strings.Join(a.Instances, ", "))
	}
	return nil
}

// Heat explains a single bubble's temperature.
func Heat(cfg config.Config, id string) error {
	var v domain.BubbleView
	if err := getJSON(cfg.ServerURL+"/api/bubbles/"+id+"/heat", cfg.ActiveToken(), &v); err != nil {
		return err
	}
	fmt.Printf("%s  %s\n", icon(v.Lifecycle), v.Name)
	fmt.Printf("  lifecycle : %s\n", v.Lifecycle)
	fmt.Printf("  score     : %.3f\n", v.Score)
	fmt.Printf("  reason    : %s\n", v.Reason)
	if v.Owner != "" {
		fmt.Printf("  owner     : %s\n", v.Owner)
	}
	if v.Outcome != "" {
		fmt.Printf("  outcome   : %s\n", v.Outcome)
	}
	fmt.Printf("  threads   : %d\n", v.Threads)
	return nil
}
