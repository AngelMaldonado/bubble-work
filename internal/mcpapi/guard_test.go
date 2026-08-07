package mcpapi

import "testing"

// The name is the guard on an irreversible act, and it is the same shape
// toggle_todo uses for a todo's text: an id alone is a question the caller
// cannot check its own answer to.
func TestSameNameGuardsADelete(t *testing.T) {
	// Spacing and case are cosmetic — an agent that re-typed the name from a
	// rendered page should not be punished for it.
	for _, claimed := range []string{"Kiosko", "kiosko", "  Kiosko  ", "Kiosko"} {
		if err := sameName(claimed, "Kiosko", "bubble"); err != nil {
			t.Errorf("a matching name was refused (%q): %v", claimed, err)
		}
	}

	// Naming something ELSE is exactly the failure this exists to catch: a
	// transposed or hallucinated id pointing at a stranger's work.
	err := sameName("Kiosko", "Login y Perfil", "bubble")
	if err == nil {
		t.Fatal("deleting a bubble the caller misnamed was allowed")
	}
	// The message has to say what it actually is, or the agent cannot recover.
	if got := err.Error(); !contains(got, "Login y Perfil") || !contains(got, "Kiosko") {
		t.Errorf("the refusal does not say what was expected vs found: %s", got)
	}

	// Omitting it is not a shortcut.
	if err := sameName("", "Kiosko", "thread"); err == nil {
		t.Error("deleting without naming anything was allowed")
	} else if !contains(err.Error(), "Kiosko") {
		t.Errorf("the refusal should tell the caller what to pass: %s", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
