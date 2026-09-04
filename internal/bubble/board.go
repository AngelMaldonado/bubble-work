package bubble

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/heat"
)

// The board: what floats, and why.
//
// Both axes are computed here and stored nowhere. Heat says how ALIVE something
// is (evidence × time); priority says how much it MATTERS (impact × urgency).
// They are independent on purpose — folded together, an untouched P1 keeps
// floating and stops looking like a problem, which is the most useful signal the
// system can produce.
func registerBoard(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/workspaces/{id}/board", func(e *core.RequestEvent) error {
			ws, _, err := reachWorkspace(e, e.Request.PathValue("id"))
			if err != nil {
				return err
			}
			b, err := BoardFor(e.App, ws)
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, b)
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}

// ThreadHeat is one thread as the board reports it: both axes, side by side and
// never mixed.
type ThreadHeat struct {
	ID       string      `json:"id"`
	Seq      int         `json:"seq"`
	Name     string      `json:"name"`
	Bubble   string      `json:"bubble,omitempty"`
	Priority string      `json:"priority,omitempty"`
	Heat     heat.Result `json:"heat"`
	Pulse    bool        `json:"pulse"`
}

// BubbleHeat is one bubble, banded by its hottest OPEN thread.
type BubbleHeat struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Owner   string      `json:"owner,omitempty"`
	Outcome string      `json:"outcome,omitempty"`
	Closed  bool        `json:"closed"`
	Heat    heat.Result `json:"heat"`
}

// Board is the whole answer, including the calibration it was computed against —
// so a reader can tell a cold thread from a short cycle.
type Board struct {
	Workspace string       `json:"workspace"`
	At        string       `json:"at"`
	Tuning    heat.Tuning  `json:"tuning"`
	Bubbles   []BubbleHeat `json:"bubbles"`
	Threads   []ThreadHeat `json:"threads"`
}

// BoardFor computes both axes for a workspace, storing nothing.
func BoardFor(app core.App, ws *core.Record) (Board, error) {
	tun, err := tuningOf(app)
	if err != nil {
		return Board{}, fmt.Errorf("no calibration: %w", err)
	}
	now := time.Now().UTC()

	rows, err := app.FindAllRecords("thread_evidence", dbx.HashExp{"workspace": ws.Id})
	if err != nil {
		return Board{}, err
	}

	prio := priorityOf(app, ws.Id)
	byBubble := map[string][]heat.Result{}
	threads := make([]ThreadHeat, 0, len(rows))

	for _, r := range rows {
		ev := heat.Evidence{
			LastWarmAt: parseTS(r.GetString("last_warm_at")),
			LastAnyAt:  parseTS(r.GetString("last_any_at")),
			WarmCount:  r.GetInt("warm_count"),
			CreatedAt:  r.GetDateTime("created").Time(),
			Completed:  completedState(app, r.GetString("state")),
		}
		res := heat.Classify(ev, tun, now)
		b := r.GetString("bubble")
		byBubble[b] = append(byBubble[b], res)
		threads = append(threads, ThreadHeat{
			ID: r.Id, Seq: r.GetInt("seq"), Name: r.GetString("name"),
			Bubble: b, Priority: prio[r.Id], Heat: res,
			Pulse: heat.HasPulse(ev, tun, now),
		})
	}
	// Hottest first, and within a band the buoyancy score orders it — the model's
	// whole claim, made spatial.
	sort.SliceStable(threads, func(i, j int) bool {
		return floats(threads[i].Heat) > floats(threads[j].Heat)
	})

	bubbles, _ := app.FindAllRecords("bubbles", dbx.HashExp{"workspace": ws.Id})
	out := make([]BubbleHeat, 0, len(bubbles))
	for _, b := range bubbles {
		closed := !b.GetDateTime("closed_at").IsZero()
		res := heat.RollUp(byBubble[b.Id], b.GetString("owner") != "", tun)
		if closed {
			res = heat.Result{Lifecycle: heat.Closed, Code: heat.ReasonClosed,
				Reason: "outcome reached or explicitly abandoned"}
		}
		out = append(out, BubbleHeat{
			ID: b.Id, Name: b.GetString("name"), Owner: b.GetString("owner"),
			Outcome: b.GetString("outcome"), Closed: closed, Heat: res,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return floats(out[i].Heat) > floats(out[j].Heat) })

	return Board{
		Workspace: ws.Id, At: now.Format(time.RFC3339), Tuning: tun,
		Bubbles: out, Threads: threads,
	}, nil
}

// floats turns a verdict into one number to sort by: the band first, the score
// inside it. A cold thing never outranks a warm one on score alone.
func floats(r heat.Result) float64 {
	band := map[heat.Lifecycle]float64{
		heat.Hot: 4, heat.Warm: 3, heat.Cooling: 2, heat.Dormant: 1, heat.Closed: 0,
	}
	return band[r.Lifecycle] + r.Score
}

func tuningOf(app core.App) (heat.Tuning, error) {
	rows, err := app.FindAllRecords("tuning")
	if err != nil || len(rows) == 0 {
		return heat.Tuning{}, err
	}
	r := rows[0]
	return heat.Tuning{
		CycleHours:         r.GetFloat("cycle_hours"),
		DormantCycles:      r.GetFloat("dormant_cycles"),
		DecayCycles:        r.GetFloat("decay_cycles"),
		GraceCycles:        r.GetFloat("grace_cycles"),
		OwnerlessIsDormant: r.GetBool("ownerless_is_dormant"),
	}, nil
}

func priorityOf(app core.App, workspace string) map[string]string {
	out := map[string]string{}
	rows, err := app.FindAllRecords("thread_priority", dbx.HashExp{"workspace": workspace})
	if err != nil {
		return out
	}
	for _, r := range rows {
		if p := r.GetString("priority"); p != "" {
			out[r.Id] = p
		}
	}
	return out
}

// parseTS reads the timestamp shape PocketBase stores dates in. A view hands them
// back as text, so there is nothing typed to lean on.
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000Z", "2006-01-02 15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
