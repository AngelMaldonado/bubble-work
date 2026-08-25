package mcpapi

import (
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/AngelMaldonado/bubble-work/prompts"
)

// The prompts are prose in files, so the thing worth testing is that the loader and
// the files agree — a prompt that silently fails to load is an agent working without
// the rules it was supposed to have been given.
func TestInvariant_MCP_EveryPromptLoads(t *testing.T) {
	ps := loadPrompts(prompts.FS())
	if len(ps) < 4 {
		t.Fatalf("loaded %d prompt(s); the set is meant to cover the saga, birth, work and finish", len(ps))
	}
	names := map[string]prompt{}
	for _, p := range ps {
		if p.title == "" || p.description == "" {
			t.Errorf("%s: a prompt with no title or description is invisible in a client's list", p.name)
		}
		if p.name != strings.ToLower(p.name) || strings.Contains(p.name, " ") {
			t.Errorf("%s: name should be a lowercase slug", p.name)
		}
		names[p.name] = p
	}
	for _, want := range []string{"bubble-work", "create-a-thread", "work-a-thread", "finish-a-thread"} {
		if _, ok := names[want]; !ok {
			t.Errorf("missing prompt %q", want)
		}
	}

	// The rules that cost a real session an afternoon must actually be in the text.
	saga := names["bubble-work"].body
	for _, claim := range []string{"Definition of Done", "comment"} {
		if !strings.Contains(saga, claim) {
			t.Errorf("the saga prompt never mentions %q", claim)
		}
	}
	if !strings.Contains(names["work-a-thread"].body, "regions") {
		t.Error("the work prompt does not tell an agent which field to quote from")
	}
}

// Placeholders resolve, optional blocks disappear when unused, and nothing leaks a
// literal {{name}} into an agent's context.
func TestPromptRendering(t *testing.T) {
	p, err := parsePrompt("t.md", "---\nname: t\ntitle: T\ndescription: d\nargs: id (required), extra\n---\n"+
		"Work on {{id}}.{{#extra}} Also: {{extra}}.{{/extra}} Done.")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.args) != 2 || !p.args[0].Required || p.args[1].Required {
		t.Fatalf("args parsed as %+v", p.args)
	}

	got := p.render(map[string]string{"id": "abc"})
	if want := "Work on abc. Done."; got != want {
		t.Errorf("without the optional arg:\n got %q\nwant %q", got, want)
	}
	got = p.render(map[string]string{"id": "abc", "extra": "be careful"})
	if want := "Work on abc. Also: be careful. Done."; got != want {
		t.Errorf("with the optional arg:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(p.render(nil), "{{") {
		t.Errorf("an unsupplied placeholder leaked: %q", p.render(nil))
	}
}

// A malformed file is skipped, not fatal: a typo in prose must never take down the
// tool surface (this package has already learned that once).
func TestMalformedPromptIsSkipped(t *testing.T) {
	if _, err := parsePrompt("no-front.md", "just a body"); err == nil {
		t.Error("a file with no frontmatter should be rejected")
	}
	if _, err := parsePrompt("no-name.md", "---\ntitle: T\n---\nbody"); err == nil {
		t.Error("a file with no name should be rejected")
	}
	if _, err := parsePrompt("no-body.md", "---\nname: t\n---\n"); err == nil {
		t.Error("a file with no body should be rejected")
	}
}

// The schema is where a closed set of values belongs. A real agent session sent a
// type the runtime rejects, because the schema advertised a free-text string —
// discovering a constraint by being refused is the failure this prevents.
func TestInvariant_MCP_ClosedSetsAreEnums(t *testing.T) {
	s := schemaFor[struct {
		Type   string `json:"type,omitempty"`
		Region string `json:"region,omitempty"`
	}](func(s *jsonschema.Schema) {
		enumOn(s, "type", "bug", "feature", "chore")
		enumOn(s, "region", "logbook", "dod")
	})
	if got := s.Properties["type"].Enum; len(got) != 3 {
		t.Errorf("type enum = %v", got)
	}
	if got := s.Properties["region"].Enum; len(got) != 2 {
		t.Errorf("region enum = %v", got)
	}
	// A field that does not exist is ignored rather than invented, so a rename in
	// the DTO cannot silently produce an enum on nothing.
	enumOn(s, "nope", "a")
	if _, ok := s.Properties["nope"]; ok {
		t.Error("enumOn invented a property")
	}
}
