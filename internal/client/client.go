// Package client is the thin human-facing CLI client. It talks ONLY to the
// Bubble Work server (never to Plane) and renders the buoyancy view.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
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
	req, err := http.NewRequest(http.MethodPost, cfg.ActiveServer()+path, buf)
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

// Tick asks the server to sweep for cooling bubbles now.
func Tick(cfg config.Config) error {
	var res struct {
		New int `json:"new_notifications"`
	}
	if err := postJSON(cfg, "/api/tick", nil, &res); err != nil {
		return err
	}
	fmt.Printf("swept — %d new notification(s)\n", res.New)
	return nil
}

// Notifications lists your cooling/dormant alerts with per-person read state.
func Notifications(cfg config.Config) error {
	var box domain.Inbox
	if err := getJSON(cfg.ActiveServer()+"/api/notifications", cfg.ActiveToken(), &box); err != nil {
		return err
	}
	if !box.Enabled {
		fmt.Println("notifications are off — turn them on with `bubble notifications on`.")
		return nil
	}
	if len(box.Notifications) == 0 {
		fmt.Println("inbox empty — nothing is sinking (run `bubble tick` to sweep now).")
		return nil
	}
	for _, n := range box.Notifications {
		mark := "  "
		if n.Unread {
			mark = "• "
		}
		fmt.Printf("%s%-5d %s  %s\n", mark, n.ID, n.At, n.Message)
	}
	fmt.Printf("\n%d unread. Mark read with `bubble notifications read <id|all>`.\n", box.UnreadCount)
	return nil
}

// MarkRead marks notifications read (ids, or all).
func MarkRead(cfg config.Config, ids []int64, all bool) error {
	body := map[string]any{"all": all}
	if !all {
		body["ids"] = ids
	}
	var res struct {
		Unread int `json:"unread_count"`
	}
	if err := postJSON(cfg, "/api/notifications/read", body, &res); err != nil {
		return err
	}
	fmt.Printf("marked read — %d unread remaining\n", res.Unread)
	return nil
}

// SetNotifyPref opts in/out of notifications.
func SetNotifyPref(cfg config.Config, enabled bool) error {
	if err := postJSON(cfg, "/api/notifications/prefs", map[string]bool{"enabled": enabled}, nil); err != nil {
		return err
	}
	if enabled {
		fmt.Println("notifications on")
	} else {
		fmt.Println("notifications off")
	}
	return nil
}

// postTok POSTs with an explicit bearer token (used by admin commands).
func postTok(cfg config.Config, path, token string, out any) error {
	req, err := http.NewRequest(http.MethodPost, cfg.ActiveServer()+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("forbidden — not a service admin (set BUBBLE_ADMIN_TOKEN or use an admin email)")
	}
	if resp.StatusCode >= 300 {
		return serverError(resp, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// kioskToken mirrors the server's store.KioskToken JSON contract.
type kioskToken struct {
	Token     string `json:"token"`
	Instance  string `json:"instance"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// adminSend performs an authenticated admin request with an optional JSON body.
func adminSend(cfg config.Config, method, path, token string, body, out any) error {
	var b []byte
	if body != nil {
		var err error
		if b, err = json.Marshal(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, cfg.ActiveServer()+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("forbidden — not a service admin (set BUBBLE_ADMIN_TOKEN or use an admin email)")
	}
	if resp.StatusCode >= 300 {
		return serverError(resp, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// AdminKioskList prints all kiosk display tokens (service admin).
func AdminKioskList(cfg config.Config, token string) error {
	var ks []kioskToken
	if err := getJSON(cfg.ActiveServer()+"/api/admin/kiosk", token, &ks); err != nil {
		return err
	}
	if len(ks) == 0 {
		fmt.Println("(no kiosk tokens)")
		return nil
	}
	for _, k := range ks {
		fmt.Printf("%-30s  instance=%s  name=%s  created=%s\n",
			k.Token, k.Instance, orDash(k.Name), k.CreatedAt)
	}
	return nil
}

// AdminKioskNew mints a read-only kiosk display token bound to an instance.
func AdminKioskNew(cfg config.Config, token, instance, name string) error {
	var k kioskToken
	if err := adminSend(cfg, http.MethodPost, "/api/admin/kiosk", token,
		map[string]string{"instance": instance, "name": name}, &k); err != nil {
		return err
	}
	fmt.Printf("kiosk token for %s:\n\n  %s\n\nOpen the board as:  <server>/?kiosk=%s\n",
		k.Instance, k.Token, k.Token)
	return nil
}

// AdminKioskRevoke deletes a kiosk display token.
func AdminKioskRevoke(cfg config.Config, token, kiosk string) error {
	if err := adminSend(cfg, http.MethodDelete, "/api/admin/kiosk/"+url.PathEscape(kiosk), token, nil, nil); err != nil {
		return err
	}
	fmt.Println("revoked")
	return nil
}

// AdminInstances lists all instances (service admin).
func AdminInstances(cfg config.Config, token string) error {
	var is []domain.AdminInstance
	if err := getJSON(cfg.ActiveServer()+"/api/admin/instances", token, &is); err != nil {
		return err
	}
	for _, i := range is {
		fmt.Printf("%-12s  %-30s  ws=%s  project=%s  webhook=%v  cached=%v\n",
			i.Slug, i.BaseURL, i.Workspace, orDash(i.Project), i.HasWebhook, i.Cached)
	}
	return nil
}

// AdminBubbles lists bubbles across ALL instances (service admin).
func AdminBubbles(cfg config.Config, token string) error {
	var vs []domain.BubbleView
	if err := getJSON(cfg.ActiveServer()+"/api/admin/bubbles", token, &vs); err != nil {
		return err
	}
	for _, v := range vs {
		fmt.Printf("%s  %-10s  %-26s  %-8s  %s\n", icon(v.Lifecycle), v.Instance, v.Name, v.Lifecycle, v.Reason)
	}
	return nil
}

// AdminStats prints a service-admin health snapshot.
func AdminStats(cfg config.Config, token string) error {
	var st domain.AdminStats
	if err := getJSON(cfg.ActiveServer()+"/api/admin/stats", token, &st); err != nil {
		return err
	}
	fmt.Printf("instances        : %d\n", st.Instances)
	fmt.Printf("cached instances : %d\n", st.CachedInstances)
	fmt.Printf("cached identities: %d\n", st.CachedIdents)
	fmt.Printf("revision         : %s\n", st.Revision)
	fmt.Printf("built            : %s\n", st.Built)
	fmt.Printf("started          : %s\n", st.StartedAt)
	return nil
}

// AdminRefresh flushes server caches; AdminTick forces a sweep (service admin).
func AdminRefresh(cfg config.Config, token string) error {
	if err := postTok(cfg, "/api/admin/refresh", token, nil); err != nil {
		return err
	}
	fmt.Println("caches flushed")
	return nil
}

func AdminTick(cfg config.Config, token string) error {
	var res struct {
		New int `json:"new_notifications"`
	}
	if err := postTok(cfg, "/api/admin/tick", token, &res); err != nil {
		return err
	}
	fmt.Printf("swept — %d new notification(s)\n", res.New)
	return nil
}

// CreateWorkspace creates a Plane project (Workspace) with modules enabled.
func CreateWorkspace(cfg config.Config, req domain.CreateWorkspaceRequest) error {
	var ws domain.Workspace
	if err := postJSON(cfg, "/api/workspaces", req, &ws); err != nil {
		return err
	}
	fmt.Printf("created workspace %q  [%s]\n", ws.Name, ws.Identifier)
	fmt.Printf("  project id: %s\n", ws.ID)
	fmt.Printf("  add a bubble: bubble bubble new --workspace %s:%s --name <name>\n", ws.Instance, ws.ID)
	return nil
}

// CreateBubble creates a Plane module (a Bubble) in a project.
func CreateBubble(cfg config.Config, req domain.CreateBubbleRequest) error {
	var b domain.NewBubble
	if err := postJSON(cfg, "/api/bubbles", req, &b); err != nil {
		return err
	}
	fmt.Printf("created bubble %q\n  id: %s\n", b.Name, b.ID)
	return nil
}

// Birth creates a thread in a bubble (server enforces the §3 birth rule).
func Birth(cfg config.Config, bubbleID, name, brief, logbook string, small bool) error {
	req := domain.BirthRequest{
		BubbleID: bubbleID, Name: name, Brief: brief, Logbook: logbook, SmallThread: small,
	}
	var res domain.BirthResult
	if err := postJSON(cfg, "/api/threads/birth", req, &res); err != nil {
		return err
	}
	if res.Created {
		fmt.Printf("born: %s\n", res.Message)
		fmt.Printf("  thread id: %s\n", res.ThreadID)
	} else {
		fmt.Println(res.Message)
	}
	return nil
}

// Review / Unreview set or clear a bubble's reviewed stage (§ web-ui).
func Review(cfg config.Config, id string) error {
	if err := postJSON(cfg, "/api/bubbles/"+id+"/review", nil, nil); err != nil {
		return err
	}
	fmt.Printf("marked reviewed: %s\n", id)
	return nil
}

func Unreview(cfg config.Config, id string) error {
	if err := postJSON(cfg, "/api/bubbles/"+id+"/unreview", nil, nil); err != nil {
		return err
	}
	fmt.Printf("review cleared: %s\n", id)
	return nil
}

// Search fuzzy-lists threads (tasks) matching q, across your instances.
func Search(cfg config.Config, q string) error {
	var hits []domain.ThreadHit
	if err := getJSON(cfg.ActiveServer()+"/api/threads?q="+url.QueryEscape(q), cfg.ActiveToken(), &hits); err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("no matching threads")
		return nil
	}
	for _, h := range hits {
		state := "done"
		if h.Open {
			state = "open"
		}
		fmt.Printf("%-5s  %-30s  %s · %s\n", state, trunc(h.Name, 30), h.BubbleName, h.Instance)
	}
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

// levelIcon / levelLabel render the 5 UI bands (§ web-ui) in the CLI.
func levelIcon(level string) string {
	switch level {
	case "in_progress":
		return "🔥"
	case "reviewed":
		return "👀"
	case "zzzz":
		return "😴"
	case "rip":
		return "🪦"
	case "done":
		return "🏆"
	default:
		return "•"
	}
}

func levelLabel(level string) string {
	if level == "in_progress" {
		return "wip"
	}
	return level
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
	if err := getJSON(cfg.ActiveServer()+"/api/bubbles", cfg.ActiveToken(), &vs); err != nil {
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

	row("  ", "INSTANCE", "LEVEL", "BUBBLE", "ID", "WHY")
	row("  ", strings.Repeat("-", iw), "-----", "------", strings.Repeat("-", dw), "---")
	for _, v := range vs {
		row(levelIcon(v.Level), v.Instance, levelLabel(v.Level), v.Name, shortID(v.ID), v.Reason)
	}
	return nil
}

// Whoami shows the identity the server resolved from your credential — proof
// that Plane pass-through auth worked, and which instances/role Plane granted.
func Whoami(cfg config.Config) error {
	var a domain.Actor
	if err := getJSON(cfg.ActiveServer()+"/api/whoami", cfg.ActiveToken(), &a); err != nil {
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

// Show renders a bubble's thread timeline newest-first — the git-log-oneline
// view of how the bubble has progressed (INTERIOR-PLAN.md Phase 10). The id may
// be short or full, like `heat`.
func Show(cfg config.Config, bubble string) error {
	var nodes []domain.ThreadNode
	if err := getJSON(cfg.ActiveServer()+"/api/bubbles/"+url.PathEscape(bubble)+"/threads", cfg.ActiveToken(), &nodes); err != nil {
		return err
	}
	if len(nodes) == 0 {
		fmt.Println("no threads in this bubble yet — birth one with `bubble birth <bubble>`")
		return nil
	}

	sw, ow := 1, 0
	for _, n := range nodes {
		sw = max(sw, len(fmt.Sprintf("%d", n.Seq)))
		ow = max(ow, utf8.RuneCountInString(n.Owner))
	}
	ow = min(ow, 16)
	used := 8 + 2 + 2 + sw + 2 + 2 + ow + 2 + 8 // hash + mark + '#'+seq + gaps + owner + age
	tw := max(termWidth()-used, 16)

	for _, n := range nodes {
		mark := "•"
		if !n.Active {
			mark = "✓"
		}
		hash := shortID(n.ID)
		if len(hash) > 8 {
			hash = hash[:8]
		}
		fmt.Printf("%-8s  %s #%-*d  %s  %s  %s\n",
			hash, mark, sw, n.Seq, pad(trunc(n.Title, tw), tw), pad(trunc(n.Owner, ow), ow), relAge(n.CreatedAt))
	}
	fmt.Printf("\n%d thread(s) · open one: bubble thread <id>\n", len(nodes))
	return nil
}

// Thread prints a thread's interior — its work artifacts, logbook (with DoD),
// and revisions (INTERIOR-PLAN.md Phase 11). With comments, it appends the feed.
func Thread(cfg config.Config, id string, comments bool) error {
	var d domain.ThreadDetail
	if err := getJSON(cfg.ActiveServer()+"/api/threads/"+url.PathEscape(id), cfg.ActiveToken(), &d); err != nil {
		return err
	}

	state := "open"
	if !d.Active {
		state = "done"
	}
	fmt.Printf("#%d  %s  [%s · %s]\n", d.Seq, d.Title, d.Kind, state)
	if len(d.Assignees) > 0 {
		fmt.Printf("assignees : %s\n", strings.Join(d.Assignees, ", "))
	}
	if d.Priority != "" && d.Priority != "none" {
		fmt.Printf("priority  : %s\n", d.Priority)
	}

	for _, a := range d.Artifacts {
		fmt.Printf("\n%s\n%s\n", heading(a.Title), a.Markdown)
	}

	if d.Logbook != nil {
		fmt.Printf("\n%s\n", heading("Logbook"))
		printTodos(d.Logbook.Todos)
		if len(d.Logbook.DoD) > 0 {
			fmt.Printf("\n%s\n", heading("Definition of Done"))
			printTodos(d.Logbook.DoD)
		}
	}

	if len(d.Revisions) > 0 {
		fmt.Printf("\n%s\n", heading("Revisions"))
		for _, rv := range d.Revisions {
			fmt.Printf("\n• %s\n%s\n", rv.Title, indent(rv.Markdown, "  "))
		}
	}

	if comments {
		return threadComments(cfg, d.ID)
	}
	fmt.Printf("\n(see the discussion with `bubble thread %s --comments`)\n", id)
	return nil
}

func threadComments(cfg config.Config, id string) error {
	var cs []domain.Comment
	if err := getJSON(cfg.ActiveServer()+"/api/threads/"+url.PathEscape(id)+"/comments", cfg.ActiveToken(), &cs); err != nil {
		return err
	}
	fmt.Printf("\n%s\n", heading("Comments"))
	if len(cs) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	var toRead []string
	for _, c := range cs {
		who := c.Author
		if who == "" {
			who = "someone"
		}
		fmt.Printf("\n%s · %s\n%s\n", who, relAge(c.CreatedAt), indent(c.Markdown, "  "))
		if len(c.Readers) > 0 {
			names := make([]string, len(c.Readers))
			for i, r := range c.Readers {
				names[i] = r.Name
			}
			fmt.Printf("  👀 %s\n", strings.Join(names, ", "))
		}
		if !c.Mine {
			toRead = append(toRead, c.ID)
		}
	}
	// Viewing marks others' comments as read (best-effort — never block output).
	if len(toRead) > 0 {
		_ = postJSON(cfg, "/api/threads/"+url.PathEscape(id)+"/comments/read",
			map[string][]string{"comment_ids": toRead}, nil)
	}
	return nil
}

// Comment posts a comment to a thread's discussion. It is written to Plane as
// you (impersonation via your Plane key), and does not warm the bubble.
func Comment(cfg config.Config, id, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("empty comment")
	}
	var c domain.Comment
	if err := postJSON(cfg, "/api/threads/"+url.PathEscape(id)+"/comments", map[string]string{"body": body}, &c); err != nil {
		return err
	}
	fmt.Printf("posted · %s · %s\n", c.Author, relAge(c.CreatedAt))
	return nil
}

func printTodos(todos []md.Todo) {
	for _, t := range todos {
		box := "[ ]"
		if t.Done {
			box = "[x]"
		}
		fmt.Printf("  %s %s\n", box, t.Text)
	}
}

// heading underlines a section title with a box-drawing rule.
func heading(s string) string {
	return s + "\n" + strings.Repeat("─", utf8.RuneCountInString(s))
}

// indent prefixes every line of s with prefix.
func indent(s, prefix string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// relAge renders a coarse "2d ago"-style age.
func relAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// Heat explains a single bubble's temperature.
func Heat(cfg config.Config, id string) error {
	var v domain.BubbleView
	if err := getJSON(cfg.ActiveServer()+"/api/bubbles/"+id+"/heat", cfg.ActiveToken(), &v); err != nil {
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
