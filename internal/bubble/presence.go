package bubble

import (
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Who is here right now.
//
// A heartbeat, and deliberately nothing else. The browser says "still here"
// while its tab is visible; this stamps the time and forgets everything older.
// What "online" MEANS is decided when the row is read — under two minutes is
// here, under fifteen is a tab left open — because a stored verdict about time
// is a verdict that goes stale in exactly the way this one does.
//
// Presence is NEVER evidence. It emits no event, warms nothing, and cannot move
// a bubble between bands. The model measures changed reality; a person looking
// at a screen has changed nothing, and a heartbeat that warmed anything would be
// the most efficient way ever built to manufacture activity.
//
// One route rather than letting the client write the collection directly: an
// upsert is a find-then-create-or-patch, which from a browser is two round trips
// and a race, and the TIME belongs to the server. A client that stamps its own
// clock is a client that can be online tomorrow.
func registerPresence(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/presence", func(e *core.RequestEvent) error {
			who := e.Auth
			if who == nil {
				return e.UnauthorizedError("sign in first", nil)
			}
			row, err := e.App.FindFirstRecordByFilter(
				"presence", "user = {:u}", dbx.Params{"u": who.Id},
			)
			if err != nil {
				col, err := e.App.FindCollectionByNameOrId("presence")
				if err != nil {
					return e.InternalServerError(err.Error(), err)
				}
				row = core.NewRecord(col)
				row.Set("user", who.Id)
			}
			row.Set("at", time.Now().UTC())
			if err := e.App.Save(row); err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]string{"at": row.GetDateTime("at").String()})
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}
