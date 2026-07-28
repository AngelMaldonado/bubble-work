// Package client is the thin human-facing CLI client. It talks ONLY to the
// Bubble Work server (never to Plane) and renders the buoyancy view.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// termWidth returns the terminal width, or a very large value when stdout is
// not a terminal (piped) so output is printed untruncated.
func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 1 << 20
}

// pad right-pads to w display columns (rune count).
func pad(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// shortID is the module segment of a namespaced bubble id — compact but still a
// full UUID, so it's unambiguous and usable directly with heat/set/close/open.
func shortID(full string) string {
	return full[strings.LastIndex(full, ":")+1:]
}

// serverError extracts an {"error": ...} message from a response body.
func serverError(resp *http.Response, path string) error {
	var e struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	return fmt.Errorf("server %s: %s", path, resp.Status)
}

// trunc shortens s to w runes, adding an ellipsis when it cuts.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

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
		return fmt.Errorf("unauthorized — check `bubble use` or set your key with `bubble init --token <key>`")
	}
	if resp.StatusCode >= 300 {
		return serverError(resp, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func postJSON(cfg config.Config, path string, body, out any) error {
	var buf *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.ServerURL+path, buf)
	if err != nil {
		return err
	}
	if t := cfg.ActiveToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("unauthorized — check `bubble use` / your Plane key")
	case http.StatusForbidden:
		return fmt.Errorf("forbidden — you're not a member of that bubble's instance")
	}
	if resp.StatusCode >= 300 {
		return serverError(resp, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// SetContract sets a bubble's §4 contract (only non-nil fields change).
func SetContract(cfg config.Config, id string, in domain.ContractInput) error {
	var c domain.Contract
	if err := postJSON(cfg, "/api/bubbles/"+id+"/contract", in, &c); err != nil {
		return err
	}
	fmt.Printf("contract updated for %s\n", id)
	fmt.Printf("  outcome : %s\n", orDash(c.Outcome))
	fmt.Printf("  owner   : %s\n", orDash(c.Owner))
	fmt.Printf("  closure : %s\n", orDash(c.Closure))
	return nil
}

// Close / Reopen flip a bubble's closed flag (§5.3).
func Close(cfg config.Config, id string) error {
	if err := postJSON(cfg, "/api/bubbles/"+id+"/close", nil, nil); err != nil {
		return err
	}
	fmt.Printf("closed %s\n", id)
	return nil
}

func Reopen(cfg config.Config, id string) error {
	if err := postJSON(cfg, "/api/bubbles/"+id+"/reopen", nil, nil); err != nil {
		return err
	}
	fmt.Printf("reopened %s\n", id)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
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

	// Size columns to content (capped), then let WHY absorb the remaining width.
	iw, nw, dw := len("INSTANCE"), len("BUBBLE"), len("ID")
	for _, v := range vs {
		iw = max(iw, utf8.RuneCountInString(v.Instance))
		nw = max(nw, utf8.RuneCountInString(v.Name))
		dw = max(dw, len(shortID(v.ID)))
	}
	iw, nw = min(iw, 14), min(nw, 30)
	const stw = 8 // fits the longest state word ("dormant"/"cooling")

	used := 2 + 2 + iw + 2 + stw + 2 + nw + 2 + dw + 2 // everything left of WHY
	whyw := termWidth() - used
	showWhy := whyw >= 8

	row := func(icn, inst, state, name, id, why string) {
		line := fmt.Sprintf("%s  %s  %s  %s  %s",
			icn, pad(inst, iw), pad(state, stw), pad(trunc(name, nw), nw), pad(id, dw))
		if showWhy {
			line += "  " + trunc(why, whyw)
		}
		fmt.Println(strings.TrimRight(line, " "))
	}

	row("  ", "INSTANCE", "STATE", "BUBBLE", "ID", "WHY")
	row("  ", strings.Repeat("-", iw), "-----", "------", strings.Repeat("-", dw), "---")
	for _, v := range vs {
		row(icon(v.Lifecycle), v.Instance, string(v.Lifecycle), v.Name, shortID(v.ID), v.Reason)
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
