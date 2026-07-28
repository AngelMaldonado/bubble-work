// Package client is the thin human-facing CLI client. It talks ONLY to the
// Bubble Work server (never to Plane) and renders the buoyancy view.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

func getJSON(url string, out any) error {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
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
	if err := getJSON(cfg.ServerURL+"/api/bubbles", &vs); err != nil {
		return err
	}
	if len(vs) == 0 {
		fmt.Println("No bubbles yet. Configure Plane with `bubble init`, then create a Module in Plane.")
		return nil
	}
	for _, v := range vs {
		fmt.Printf("%s  %-28s  %-8s  %s\n", icon(v.Lifecycle), v.Name, v.Lifecycle, v.Reason)
	}
	return nil
}

// Heat explains a single bubble's temperature.
func Heat(cfg config.Config, id string) error {
	var v domain.BubbleView
	if err := getJSON(cfg.ServerURL+"/api/bubbles/"+id+"/heat", &v); err != nil {
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
