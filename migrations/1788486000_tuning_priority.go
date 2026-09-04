package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// The calibration, and the first of the two derived axes.
//
// `tuning` is a ROW, not a constant and not part of any view's SQL. That is what
// makes recalibrating an UPDATE instead of a migration: the views below join it,
// so the SELECT text is fixed while the numbers are data. (Written down because
// the plan once claimed the opposite, and the opposite is false — it was tested.)
func init() {
	m.Register(func(app core.App) error {
		tun := core.NewBaseCollection("tuning")
		// The rolling heat window. Everything else is expressed in cycles, so this
		// is the one number that has a unit.
		tun.Fields.Add(&core.NumberField{Name: "cycle_hours", Required: true})
		// How many cycles of silence make something Dormant. 2 = "nothing this
		// cycle or last".
		tun.Fields.Add(&core.NumberField{Name: "dormant_cycles", Required: true})
		// Scales the score's exponential decay, in cycles. Larger = things stay
		// buoyant longer.
		tun.Fields.Add(&core.NumberField{Name: "decay_cycles", Required: true})
		// A thread born less than this ago, with nothing yet, reads as new rather
		// than as neglected.
		tun.Fields.Add(&core.NumberField{Name: "grace_cycles", Required: true})
		// Sinks anything with nobody accountable, ONCE it has gone quiet. Something
		// still producing stays hot either way — output outranks paperwork.
		tun.Fields.Add(&core.BoolField{Name: "ownerless_is_dormant"})
		tun.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

		// Everyone reads the calibration — it explains what they are looking at.
		// Only the global lead changes it, because it changes what the whole
		// organisation sees at once.
		tun.ListRule = types.Pointer(`@request.auth.id != ""`)
		tun.ViewRule = types.Pointer(`@request.auth.id != ""`)
		tun.UpdateRule = types.Pointer(globalLead)
		tun.CreateRule = nil
		tun.DeleteRule = nil
		if err := app.Save(tun); err != nil {
			return err
		}

		// One row, seeded. A view that joins an empty table returns nothing, so the
		// board would go blank rather than uncalibrated.
		row := core.NewRecord(tun)
		row.Set("cycle_hours", 168)  // one week
		row.Set("dormant_cycles", 2) // nothing this cycle or last
		row.Set("decay_cycles", 2)   // ≈0.37 after one cycle, ≈0.14 after two
		row.Set("grace_cycles", 1)
		row.Set("ownerless_is_dormant", true)
		if err := app.Save(row); err != nil {
			return err
		}

		// ---- priority: derived, never stored ----
		//
		// The determinant map as SQL. `priority` is not a column anywhere: a stored
		// verdict is one somebody can write inconsistently with the map, which turns
		// the map into decoration.
		//
		// A view is the right home because it is a pure function of two columns —
		// no clock, no calibration — so the planner gets filtering and sorting from
		// the standard API with no endpoint written.
		pri := core.NewViewCollection("thread_priority")
		// Two things this SELECT has to survive, both learned the hard way.
		//
		// One line and parenthesised: PocketBase parses a view's SELECT list itself
		// to derive the collection's fields, and a bare multi-line CASE comes back
		// as `invalid identifier parts`.
		//
		// And CAST: without it a computed column is typed `json`, so the value
		// arrives quoted and `priority = 'P1'` filters through JSON_EXTRACT. The
		// cast is what makes it an ordinary text field.
		pri.ViewQuery = "SELECT t.id AS id, t.workspace AS workspace, t.bubble AS bubble," +
			" t.seq AS seq, t.name AS name, t.impact AS impact, t.urgency AS urgency," +
			" CAST(CASE" +
			"   WHEN t.impact = '' OR t.urgency = '' THEN ''" +
			"   WHEN t.impact = 'high' AND t.urgency = 'high' THEN 'P1'" +
			"   WHEN t.impact = 'high' AND t.urgency = 'mid'  THEN 'P2'" +
			"   WHEN t.impact = 'high' AND t.urgency = 'low'  THEN 'P3'" +
			"   WHEN t.impact = 'mid'  AND t.urgency = 'high' THEN 'P2'" +
			"   WHEN t.impact = 'mid'  AND t.urgency = 'mid'  THEN 'P2'" +
			"   WHEN t.impact = 'mid'  AND t.urgency = 'low'  THEN 'P3'" +
			"   WHEN t.impact = 'low'  AND t.urgency = 'high' THEN 'P3'" +
			"   WHEN t.impact = 'low'  AND t.urgency = 'mid'  THEN 'P3'" +
			"   ELSE 'P4'" +
			" END AS TEXT) AS priority" +
			" FROM threads t"
		seen := orLead(`workspace.memberships_via_workspace.user ?= @request.auth.id`)
		pri.ListRule = types.Pointer(seen)
		pri.ViewRule = types.Pointer(seen)
		return app.Save(pri)
	}, func(app core.App) error {
		for _, name := range []string{"thread_priority", "tuning"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
