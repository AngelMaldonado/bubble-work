package main

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// La limpieza borra a las simuladas y todo lo que apunta a ellas, y no toca a
// nadie más: ni sus filas, ni su lugar en una relación compartida.
func TestPurgeSimulated(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.ResetBootstrapState()
	if _, err := core.NewMigrationsRunner(app, core.AppMigrations).Up(); err != nil {
		t.Fatal(err)
	}

	save := func(r *core.Record) *core.Record {
		t.Helper()
		if err := app.Save(r); err != nil {
			t.Fatalf("%s: %v", r.Collection().Name, err)
		}
		return r
	}
	col := func(name string) *core.Collection {
		c, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	person := func(email, role string) *core.Record {
		r := core.NewRecord(col("users"))
		r.Set("email", email)
		r.SetPassword("passwordpass")
		r.Set("role", role)
		return save(r)
	}
	real := person("angel@cuby.mx", "lead")
	sim := person("ana@simulado.test", "member")
	simLead := person("lead@simulado.test", "lead")
	// Un correo que CONTIENE el dominio sin terminar en él no es simulado.
	lookalike := person("x@simulado.test.mx", "member")

	token := func(owner *core.Record) *core.Record {
		r := core.NewRecord(col("agent_tokens"))
		r.Set("owner", owner.Id)
		r.Set("name", "agente")
		r.Set("hash", owner.Id+"hash")
		return save(r)
	}
	token(sim)
	keep := token(real)
	agents := func(author *core.Record, content string) *core.Record {
		r := core.NewRecord(col("agents_md"))
		r.Set("role", "operators")
		r.Set("content", content)
		r.Set("author", author.Id)
		return save(r)
	}
	agents(real, "# real")
	agents(simLead, "# simulada")

	// Una relación de varias personas: la simulada sale de la lista, la fila se
	// queda con las demás.
	teams := col("teams")
	team := core.NewRecord(teams)
	team.Set("name", "Equipo")
	save(team)
	ws := core.NewRecord(col("workspaces"))
	ws.Set("name", "W")
	ws.Set("slug", "w")
	ws.Set("team", team.Id)
	save(ws)
	thread := core.NewRecord(col("threads"))
	thread.Set("workspace", ws.Id)
	thread.Set("name", "Hilo")
	thread.Set("assignees", []string{real.Id, sim.Id})
	save(thread)

	people, _, err := purgeSimulated(app)
	if err != nil {
		t.Fatal(err)
	}
	if people != 2 {
		t.Fatalf("borró %d personas, esperaba 2", people)
	}
	for _, id := range []string{sim.Id, simLead.Id} {
		if _, err := app.FindRecordById("users", id); err == nil {
			t.Fatalf("la simulada %s sigue", id)
		}
	}
	for _, id := range []string{real.Id, lookalike.Id} {
		if _, err := app.FindRecordById("users", id); err != nil {
			t.Fatalf("borró a %s, que no es simulada", id)
		}
	}
	if n, _ := app.CountRecords("agent_tokens"); n != 1 {
		t.Fatalf("quedan %d tokens, esperaba sólo el real", n)
	}
	if _, err := app.FindRecordById("agent_tokens", keep.Id); err != nil {
		t.Fatal("borró el token de una persona real")
	}
	rows, _ := app.FindAllRecords("agents_md")
	if len(rows) != 1 || rows[0].GetString("content") != "# real" {
		t.Fatalf("agents_md: %d filas, esperaba sólo la real", len(rows))
	}
	th, err := app.FindRecordById("threads", thread.Id)
	if err != nil {
		t.Fatal("borró un hilo con asignados reales")
	}
	if got := th.GetStringSlice("assignees"); len(got) != 1 || got[0] != real.Id {
		t.Fatalf("assignees = %v", got)
	}

	// Otra vez: no hay nada que hacer, y no es un error.
	if people, rows, err := purgeSimulated(app); err != nil || people != 0 || rows != 0 {
		t.Fatalf("segunda pasada: %d personas, %d filas, %v", people, rows, err)
	}
}
