package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// El relleno de 1788538000_thread_team y de 1788537000_thread_counters sobre
// datos que ya existían: se aplican todas las migraciones, se revierten esas
// dos, se crean hilos como antes y se vuelven a aplicar.
func TestThreadTeamAndCounterBackfill(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.ResetBootstrapState()
	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	// Hasta antes del contador, sin importar cuántas migraciones vengan
	// después: todo lo revertido se vuelve a aplicar abajo.
	reverted := []string{}
	for i := 0; i < 20 && !slices.Contains(reverted, "1788537000_thread_counters.go"); i++ {
		r, err := runner.Down(1)
		if err != nil {
			t.Fatal(err)
		}
		if len(r) == 0 {
			break
		}
		reverted = append(reverted, r...)
	}
	if !slices.Contains(reverted, "1788538000_thread_team.go") || !slices.Contains(reverted, "1788537000_thread_counters.go") {
		t.Fatalf("reverted %v; expected to reach the counter migration", reverted)
	}

	teams, err := app.FindCollectionByNameOrId("teams")
	if err != nil {
		t.Fatal(err)
	}
	team := core.NewRecord(teams)
	team.Set("name", "Ops")
	if err := app.Save(team); err != nil {
		t.Fatal(err)
	}
	wsCol, err := app.FindCollectionByNameOrId("workspaces")
	if err != nil {
		t.Fatal(err)
	}
	ws := core.NewRecord(wsCol)
	ws.Set("name", "Almacén")
	ws.Set("slug", "almacen")
	ws.Set("team", team.Id)
	if err := app.Save(ws); err != nil {
		t.Fatal(err)
	}
	thCol, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []int{1, 2, 5} {
		th := core.NewRecord(thCol)
		th.Set("workspace", ws.Id)
		th.Set("name", "Hilo")
		th.Set("seq", seq)
		if err := app.Save(th); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}

	var withoutTeam int
	if err := app.DB().NewQuery("SELECT COUNT(*) FROM threads WHERE team != {:t}").
		Bind(map[string]any{"t": team.Id}).Row(&withoutTeam); err != nil {
		t.Fatal(err)
	}
	if withoutTeam != 0 {
		t.Fatalf("%d threads did not take their workspace's team", withoutTeam)
	}
	var last int
	if err := app.DB().NewQuery("SELECT last FROM thread_counters WHERE scope_kind = 'workspace' AND scope_id = {:w}").
		Bind(map[string]any{"w": ws.Id}).Row(&last); err != nil {
		t.Fatal(err)
	}
	if last != 5 {
		t.Fatalf("counter starts at %d, want the highest existing number (5)", last)
	}
}
