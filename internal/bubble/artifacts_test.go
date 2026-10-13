package bubble

import (
	"strings"
	"testing"
	"time"
)

func q(id string, round float64, deps ...string) Entry {
	d := make([]any, len(deps))
	for i, x := range deps {
		d[i] = x
	}
	return Entry{Type: "question", Data: map[string]any{
		"id": id, "round": round, "title": "t " + id, "body": "b", "recommendation": "yes", "depends_on": d,
	}}
}

func ans(id, verdict, text string) Entry {
	return Entry{Type: "answer", Via: "app", Data: map[string]any{"question": id, "verdict": verdict, "text": text}}
}

// The frontier is derived: a question is askable once everything it depends on
// is answered, and blocked until then.
func TestDeriveQuestionsFrontier(t *testing.T) {
	es := []Entry{q("Q1", 1), q("Q2", 1), q("Q3", 2, "Q1", "Q2"), ans("Q1", "accepted", "")}
	got := map[string]string{}
	for _, x := range DeriveQuestions(es) {
		got[x.ID] = x.State
	}
	want := map[string]string{"Q1": "answered", "Q2": "frontier", "Q3": "blocked"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %s, want %s", k, got[k], v)
		}
	}
	es = append(es, ans("Q2", "override", "no, b"))
	for _, x := range DeriveQuestions(es) {
		if x.ID == "Q3" && x.State != "frontier" {
			t.Errorf("Q3 should be askable once Q1 and Q2 are answered, got %s", x.State)
		}
		if x.ID == "Q2" && (x.Answer == nil || x.Answer.Text != "no, b") {
			t.Errorf("Q2 should carry its override")
		}
	}
}

// Answering again replaces the verdict: the latest answer wins.
func TestDeriveQuestionsLatestAnswerWins(t *testing.T) {
	es := []Entry{q("Q1", 1), ans("Q1", "accepted", ""), ans("Q1", "override", "changed my mind")}
	if a := DeriveQuestions(es)[0].Answer; a == nil || a.Verdict != "override" {
		t.Fatalf("latest answer should win, got %+v", a)
	}
}

func TestValidateGrill(t *testing.T) {
	prior := []Entry{q("Q1", 1)}
	cases := []struct {
		name string
		e    Entry
		want string // substring of the error; "" means accepted
	}{
		{"a new question", q("Q2", 1, "Q1"), ""},
		{"a duplicate id", q("Q1", 1), "already exists"},
		{"a dependency nobody asked", q("Q3", 2, "Q9"), "never asked"},
		{"no round", Entry{Type: "question", Data: map[string]any{"id": "Q4", "title": "t", "body": "b"}}, "round"},
		{"an answer", ans("Q1", "accepted", ""), ""},
		{"an answer to nothing", ans("Q7", "accepted", ""), "no question"},
		{"an override without text", ans("Q1", "override", ""), "needs the text"},
		{"a made-up verdict", ans("Q1", "maybe", ""), "verdict"},
		{"a hand-off through append", Entry{Type: "handoff", Data: map[string]any{}}, "own call"},
	}
	for _, c := range cases {
		err := validateGrill(c.e, prior)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: want error with %q, got %v", c.name, c.want, err)
		}
	}
}

func TestExpiresAt(t *testing.T) {
	touched := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if got := ExpiresAt(touched, false, 30); !got.Equal(touched.AddDate(0, 0, 30)) {
		t.Errorf("30 days after it last moved, got %v", got)
	}
	if !ExpiresAt(touched, true, 30).IsZero() {
		t.Error("a pinned artifact never expires")
	}
}
