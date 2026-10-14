package bubble

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// This projection is narrower than the ordinary workspace board. Membership
// is still the outer boundary; within it, operators read only their part.
func participantBoard(board Board, user string) Board {
	owned, included := map[string]bool{}, map[string]bool{}
	for _, b := range board.Bubbles {
		if slices.Contains(b.Owners, user) {
			owned[b.ID], included[b.ID] = true, true
		}
	}
	for _, t := range board.Threads {
		if slices.Contains(t.Assignees, user) && t.Bubble != "" {
			included[t.Bubble] = true
		}
	}
	bs, ts, ws := []BubbleHeat{}, []ThreadHeat{}, []BoardWorkspace{}
	used := map[string]bool{}
	for _, b := range board.Bubbles {
		if included[b.ID] {
			bs = append(bs, b)
			used[b.Workspace] = true
		}
	}
	for _, t := range board.Threads {
		if owned[t.Bubble] || slices.Contains(t.Assignees, user) {
			ts = append(ts, t)
			used[t.Workspace] = true
		}
	}
	for _, w := range board.Workspaces {
		if used[w.ID] {
			ws = append(ws, w)
		}
	}
	board.Bubbles, board.Threads, board.Workspaces = bs, ts, ws
	return board
}

type readInboxItem struct {
	ID         string `json:"id"`
	Note       string `json:"note"`
	Body       string `json:"body"`
	CapturedBy string `json:"captured_by"`
	Created    string `json:"created"`
	SourceFrom string `json:"source_from,omitempty"`
	Team       string `json:"team"`
}

func registerReadPlanner(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/planner/read", func(e *core.RequestEvent) error {
			if !isPersonAuth(e.Auth) {
				return e.ForbiddenError("sign in as a person", nil)
			}
			board, err := AllBoards(e)
			if err != nil {
				return err
			}
			// Quien lidera un equipo ve el tablero entero de lo que alcanza; los
			// demás, lo suyo. El inbox y los artefactos: lo propio y lo de los
			// equipos que lidera, nunca el de otro equipo.
			led := LedTeams(e.App, e.Auth)
			if len(led) == 0 {
				board = participantBoard(board, e.Auth.Id)
			}
			params := dbx.Params{"u": e.Auth.Id}
			filter, artFilter := "captured_by = {:u}", "creator = {:u}"
			for i, t := range led {
				k := fmt.Sprintf("t%d", i)
				params[k] = t
				filter += " || team = {:" + k + "}"
				artFilter += " || inbox_item.team = {:" + k + "}"
			}
			rows, err := e.App.FindRecordsByFilter("inbox_items", filter, "-created", 0, 0, params)
			if err != nil {
				return e.InternalServerError("could not read inbox", err)
			}
			ars, err := e.App.FindRecordsByFilter("artifacts", artFilter, "-touched_at", 0, 0, params)
			if err != nil {
				return e.InternalServerError("could not read artifacts", err)
			}
			artifacts := []ArtifactView{}
			attached := map[string]bool{}
			for _, art := range ars {
				entries, err := loadEntries(e.App, art.Id)
				if err != nil {
					return e.InternalServerError("could not read artifact", err)
				}
				artifacts = append(artifacts, viewOf(e.App, art, entries, false))
				attached[art.GetString("inbox_item")] = true
			}
			notes := []readInboxItem{}
			for _, n := range rows {
				// Triaged notes normally leave Inbox. A surviving artifact still needs a
				// landing place from the brain, even after its work was handed off.
				if (n.GetString("thread") != "" || n.GetString("bubble") != "") && !attached[n.Id] {
					continue
				}
				notes = append(notes, readInboxItem{ID: n.Id, Note: n.GetString("note"), Body: n.GetString("body"), CapturedBy: n.GetString("captured_by"), Created: n.GetString("created"), SourceFrom: n.GetString("source_from"), Team: n.GetString("team")})
			}
			return e.JSON(http.StatusOK, map[string]any{"board": board, "inbox": notes, "artifacts": artifacts})
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}
