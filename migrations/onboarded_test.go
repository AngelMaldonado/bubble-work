package migrations

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// El relleno de 1788546000_onboarded: quien existía antes queda como recibido,
// conservando lo que ya tenía en sus preferencias; quien llega después, no.
func TestOnboardedBackfill(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	defer app.ResetBootstrapState()
	runner := core.NewMigrationsRunner(app, core.AppMigrations)
	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	reverted := []string{}
	for i := 0; i < 20 && !slices.Contains(reverted, "1788546000_onboarded.go"); i++ {
		r, err := runner.Down(1)
		if err != nil {
			t.Fatal(err)
		}
		if len(r) == 0 {
			break
		}
		reverted = append(reverted, r...)
	}
	if !slices.Contains(reverted, "1788546000_onboarded.go") {
		t.Fatalf("reverted %v; expected to reach the onboarded migration", reverted)
	}

	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	prefsCol, err := app.FindCollectionByNameOrId("prefs")
	if err != nil {
		t.Fatal(err)
	}
	person := func(email string) *core.Record {
		u := core.NewRecord(usersCol)
		u.SetEmail(email)
		u.SetPassword("contraseña-larga-1")
		u.Set("name", email)
		u.Set("role", "member")
		if err := app.Save(u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	bare := person("sin-prefs@example.com")
	withPrefs := person("con-prefs@example.com")
	row := core.NewRecord(prefsCol)
	row.Set("user", withPrefs.Id)
	row.Set("value", map[string]any{"week_start": 0})
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	if _, err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	late := person("nueva@example.com")

	read := func(u *core.Record) map[string]any {
		r, err := app.FindFirstRecordByFilter(prefsCol, "user = {:u}", map[string]any{"u": u.Id})
		if err != nil {
			return nil
		}
		v := map[string]any{}
		if err := r.UnmarshalJSONField("value", &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := read(bare); v["onboarded_at"] == nil {
		t.Errorf("a person without prefs was not marked: %v", v)
	}
	if v := read(withPrefs); v["onboarded_at"] == nil || v["week_start"] != float64(0) {
		t.Errorf("existing prefs lost or unmarked: %v", v)
	}
	if v := read(late); v["onboarded_at"] != nil {
		t.Errorf("a person created after the migration was marked: %v", v)
	}
}
