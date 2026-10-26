package fury

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC)

func rules(t *testing.T, f Facts, want int, wantRules ...string) Verdict {
	t.Helper()
	v := Evaluate(f, Defaults(), now)
	if v.Level.Level != want {
		t.Fatalf("nivel %d, esperaba %d (%+v)", v.Level.Level, want, v.Reasons)
	}
	if len(v.Reasons) != len(wantRules) {
		t.Fatalf("razones %+v, esperaba %v", v.Reasons, wantRules)
	}
	for i, r := range wantRules {
		if v.Reasons[i].Rule != r {
			t.Fatalf("razón %d = %s, esperaba %s", i, v.Reasons[i].Rule, r)
		}
	}
	return v
}

func thread(o func(*Facts)) Facts {
	f := Facts{Kind: Thread, Priority: "P1", People: 1, Lifecycle: "hot"}
	o(&f)
	return f
}

func TestZenByDefault(t *testing.T) {
	v := rules(t, thread(func(*Facts) {}), Zen)
	if v.Name != "Zen" || v.Color != "green" || v.Blink {
		t.Fatalf("%+v", v.Level)
	}
}

// R2: vence al terminar su día. El mismo día no está vencida; al día siguiente
// sí, y a los 8 días (más de 7) está desatada.
func TestOverdue(t *testing.T) {
	rules(t, thread(func(f *Facts) { f.Due = "2026-10-22" }), Zen)
	v := rules(t, thread(func(f *Facts) { f.Due = "2026-10-21" }), Furiosa, "overdue")
	if v.Reasons[0].Text != "vencida hace 1 día · P1" {
		t.Fatalf("%q", v.Reasons[0].Text)
	}
	rules(t, thread(func(f *Facts) { f.Due = "2026-10-15" }), Furiosa, "overdue") // 7 días: aún no
	v = rules(t, thread(func(f *Facts) { f.Due = "2026-10-14" }), Desatada, "overdue")
	if !v.Blink || v.Color != "red" {
		t.Fatalf("desatada parpadea en rojo: %+v", v.Level)
	}
	// Una fecha con hora, como la guarda la base, se lee igual.
	rules(t, thread(func(f *Facts) { f.Due = "2026-10-21 00:00:00.000Z" }), Furiosa, "overdue")
}

func TestOnlyHighPriority(t *testing.T) {
	for _, p := range []string{"P3", "P4", ""} {
		rules(t, thread(func(f *Facts) { f.Priority = p; f.Due = "2026-01-01"; f.People = 0; f.Lifecycle = "dormant" }), Zen)
	}
	rules(t, thread(func(f *Facts) { f.Priority = "P2"; f.Due = "2026-10-20" }), Furiosa, "overdue")
}

func TestDoneHasNoFury(t *testing.T) {
	rules(t, thread(func(f *Facts) { f.Done = true; f.Due = "2026-01-01"; f.People = 0 }), Zen)
}

func TestForgottenAndOwnerless(t *testing.T) {
	rules(t, thread(func(f *Facts) { f.Lifecycle = "dormant" }), Irritada, "forgotten")
	v := rules(t, Facts{Kind: Bubble, Priority: "P2", Lifecycle: "rip"}, Furiosa, "forgotten", "ownerless")
	if v.Reasons[1].Text != "sin responsables · P2" {
		t.Fatalf("%q", v.Reasons[1].Text)
	}
	v = rules(t, thread(func(f *Facts) { f.People = 0 }), Irritada, "ownerless")
	if v.Reasons[0].Text != "sin asignados · P1" {
		t.Fatalf("%q", v.Reasons[0].Text)
	}
}

func inflated(k int) Facts {
	return Facts{Kind: Bubble, Priority: "P4", People: 1, Lifecycle: "hot",
		Threads: 8 * k, Words: 6000 * k, ActiveDays: 10 * k, Created: now.Add(-time.Duration(21*k) * 24 * time.Hour)}
}

// R1 pide TODOS los umbrales; con el doble de todos, sube. Sin prioridad alta
// también: lo inflado es inflado.
func TestInflated(t *testing.T) {
	v := rules(t, inflated(1), Irritada, "inflated")
	if !strings.HasPrefix(v.Reasons[0].Text, "inflada: 8 hilos · 6000 palabras · 10 días activos · 21 días abierta") {
		t.Fatalf("%q", v.Reasons[0].Text)
	}
	rules(t, inflated(2), Furiosa, "inflated")
	f := inflated(2)
	f.Words = 11999 // le falta el doble de palabras
	rules(t, f, Irritada, "inflated")
	f = inflated(1)
	f.ActiveDays = 9
	rules(t, f, Zen)
	f = inflated(1)
	f.Kind = Thread // un hilo no se infla
	rules(t, f, Zen)
}

func TestRulesCanBeOff(t *testing.T) {
	r := Defaults()
	r.Overdue.On = false
	if v := Evaluate(thread(func(f *Facts) { f.Due = "2026-01-01" }), r, now); v.Level.Level != Zen {
		t.Fatalf("una regla apagada no dispara: %+v", v)
	}
}

// El máximo de las causas, con todas: la más grave primero.
func TestMaxOfCauses(t *testing.T) {
	rules(t, thread(func(f *Facts) { f.Due = "2026-10-01"; f.People = 0; f.Lifecycle = "dormant" }),
		Desatada, "overdue", "forgotten", "ownerless")
}

func TestMergeBubbleWithThreads(t *testing.T) {
	own := Evaluate(Facts{Kind: Bubble, Priority: "P1", Lifecycle: "hot"}, Defaults(), now) // sin responsables: 1
	hot := Evaluate(thread(func(f *Facts) { f.Due = "2026-10-01" }), Defaults(), now)       // vencida: 3
	v := Merge(own, []Verdict{hot}, []string{"t1"}, []string{"Pagos"})
	if v.Level.Level != Desatada || v.Reasons[0].ThreadName != "Pagos" || v.Reasons[1].Rule != "ownerless" || v.Reasons[1].Thread != "" {
		t.Fatalf("%+v", v)
	}
}

func TestValidate(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatal(err)
	}
	r := Defaults()
	r.Overdue.LateLevel = 4
	if r.Validate() == nil {
		t.Fatal("nivel 4 aceptado")
	}
	r = Defaults()
	r.Inflated.Words = 0
	if r.Validate() == nil {
		t.Fatal("umbral 0 aceptado")
	}
}
