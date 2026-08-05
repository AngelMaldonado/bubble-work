package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
)

// The mirror's admin surface (docs/PLANE-SYNC.md Phase 1).
//
// sync-diff and sync-backfill both walk Plane completely and are rate-budgeted
// server-side, so they can legitimately take minutes on a large workspace —
// hence their own timeout rather than the 30s the other admin calls use.
const syncTimeout = 10 * time.Minute

// AdminSync runs one of the mirror commands: sync (census), sync-diff
// (compare against Plane) or sync-backfill (force a complete re-walk).
func AdminSync(cfg config.Config, token, cmd, instance string) error {
	switch cmd {
	case "sync":
		var st domain.SyncStatus
		if err := getJSON(cfg.ActiveServer()+"/api/admin/sync/"+instance, token, &st); err != nil {
			return err
		}
		printSyncStatus(st)
		return nil
	case "sync-diff":
		var d domain.SyncDiff
		if err := postLong(cfg, "/api/admin/sync/"+instance+"/diff", token, &d); err != nil {
			return err
		}
		printSyncDiff(d)
		if !d.Clean {
			// A dirty mirror is a real failure: it is the Phase 1 gate. Exiting
			// non-zero lets this be scripted or wired into CI later.
			return fmt.Errorf("%d finding(s) — the mirror does not match Plane", len(d.Findings))
		}
		return nil
	case "sync-rebuild":
		var r domain.SyncResult
		// Destructive-looking but safe by construction: the mirror is a
		// projection, so this costs a backfill and nothing else.
		if err := postLong(cfg, "/api/admin/sync/"+instance+"/rebuild", token, &r); err != nil {
			return err
		}
		fmt.Printf("rebuilt %s from scratch: %d projects, %d modules, %d items, %d comments in %s\n",
			r.Instance, r.Projects, r.Modules, r.Items, r.Comments,
			(time.Duration(r.TookMS) * time.Millisecond).Round(time.Millisecond))
		if r.Partial {
			fmt.Printf("PARTIAL — %d project(s) incomplete; a later pass will finish them\n", len(r.Errors))
		}
		return nil
	case "sync-backfill":
		var r domain.SyncResult
		if err := postLong(cfg, "/api/admin/sync/"+instance+"/backfill", token, &r); err != nil {
			return err
		}
		fmt.Printf("%s: %d projects, %d modules, %d items, %d comments, %d pruned in %s\n",
			r.Instance, r.Projects, r.Modules, r.Items, r.Comments, r.Pruned,
			(time.Duration(r.TookMS) * time.Millisecond).Round(time.Millisecond))
		if r.Watermark != "" {
			fmt.Printf("watermark: %s\n", r.Watermark)
		}
		return nil
	}
	return fmt.Errorf("unknown sync command %q", cmd)
}

func printSyncStatus(st domain.SyncStatus) {
	fmt.Printf("mirror %s\n", st.Instance)
	fmt.Printf("  projects : %d\n", st.Projects)
	fmt.Printf("  modules  : %d\n", st.Modules)
	fmt.Printf("  items    : %d\n", st.Items)
	fmt.Printf("  states   : %d\n", st.States)
	fmt.Printf("  members  : %d\n", st.Members)
	fmt.Printf("  comments : %d\n", st.Comments)
	fmt.Println()
	fmt.Printf("  watermark: %s\n", orDash(st.Watermark))
	fmt.Printf("  last full: %s\n", orDash(st.LastFull))
	fmt.Printf("  last ok  : %s\n", orDash(st.LastOK))
	if st.LastError != "" {
		fmt.Printf("  last error: %s\n", st.LastError)
	}
	if st.Items == 0 {
		fmt.Println("\n  (nothing mirrored yet — the first sync pass runs at startup and every 2m)")
	}
}

func printSyncDiff(d domain.SyncDiff) {
	took := (time.Duration(d.TookMS) * time.Millisecond).Round(time.Millisecond)
	fmt.Printf("sync-diff %s — compared %d project(s), %d module(s), %d item(s) in %s\n",
		d.Instance, d.Projects, d.Modules, d.Items, took)
	if d.LastError != "" {
		fmt.Printf("last sync error: %s\n", d.LastError)
	}
	if d.Clean {
		fmt.Println("\n  ✓ mirror matches Plane")
		return
	}
	fmt.Printf("\n  %d finding(s):\n", len(d.Findings))
	for _, f := range d.Findings {
		fmt.Printf("    · %s\n", f.Text)
	}
	fmt.Println("\n  A stale watermark explains most field differences — try")
	fmt.Printf("  `bubble admin sync-backfill %s` and re-run.\n", d.Instance)
}

// postLong is postTok with a timeout that suits a full Plane walk.
func postLong(cfg config.Config, path, token string, out any) error {
	req, err := http.NewRequest(http.MethodPost, cfg.ActiveServer()+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: syncTimeout}).Do(req)
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

// AdminOutbox lists writes that have not reached Plane, or drops one.
func AdminOutbox(cfg config.Config, token string, args []string) error {
	if len(args) >= 2 && args[0] == "drop" {
		path := fmt.Sprintf("%s/api/admin/outbox/%s", cfg.ActiveServer(), url.PathEscape(args[1]))
		req, err := http.NewRequest(http.MethodDelete, path, nil)
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
		if resp.StatusCode >= 300 {
			return serverError(resp, path)
		}
		fmt.Printf("dropped outbox entry %s\n", args[1])
		return nil
	}

	var v domain.OutboxView
	if err := getJSON(cfg.ActiveServer()+"/api/admin/outbox", token, &v); err != nil {
		return err
	}
	fmt.Printf("outbox: %d pending, %d abandoned\n", v.Pending, v.Abandoned)
	if len(v.Entries) == 0 {
		fmt.Println("  (empty — every write has reached Plane)")
		return nil
	}
	for _, e := range v.Entries {
		mark := "⧗"
		if e.Status == "abandoned" {
			mark = "✖"
		}
		fmt.Printf("\n%s %d  %s  [%s]\n", mark, e.ID, e.Summary, e.Instance)
		fmt.Printf("   status=%s attempts=%d", e.Status, e.Attempts)
		if e.NextAt != "" {
			fmt.Printf(" next=%s", e.NextAt)
		}
		fmt.Println()
		if e.LastError != "" {
			fmt.Printf("   ↳ %s\n", e.LastError)
		}
		if e.Kind == "comment" {
			// Comment drafts carry no credential, so nothing drains them: only
			// their author can re-send, with their own key.
			fmt.Printf("   (a draft — only %s can re-send it)\n", e.Author)
		}
	}
	return nil
}
