package md

import (
	"strings"
	"testing"
)

func TestCheckboxHTMLAddressesSource(t *testing.T) {
	cases := []struct {
		name, source string
		indices      []string
	}{
		{"ordinary", "- [ ] **uno**\n- [x] dos\n", []string{"0", "1"}},
		{"fenced example", "```md\n- [ ] ejemplo\n```\n\n- [ ] real\n", []string{"1"}},
		{"nested and loose", "- [ ] padre\n  * [ ] hijo\n\n- [ ] otro\n\n  un párrafo\n", []string{"0", "1", "2"}},
		{"CRLF and Unicode", "# Título\r\n\r\n- [ ] acción\r\n- [X] después\r\n", []string{"0", "1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, render := range []func(string) string{RenderHTML, RenderThreadHTML} {
				html := render(c.source)
				if got := strings.Count(html, "data-todo-index="); got != len(c.indices) {
					t.Fatalf("%d addresses, want %d: %s", got, len(c.indices), html)
				}
				for _, index := range c.indices {
					if !strings.Contains(html, `data-todo-index="`+index+`"`) {
						t.Fatalf("missing %s: %s", index, html)
					}
				}
			}
		})
	}
	source := "```md\n- [ ] ejemplo\n```\n\n- [ ] real\n"
	changed, err := ToggleTodo(source, 1, "real", true)
	if err != nil || !strings.Contains(changed, "- [ ] ejemplo") || !strings.Contains(changed, "- [x] real") {
		t.Fatalf("wrong source changed: %s (%v)", changed, err)
	}
}

func TestCheckboxUnsupportedSourceHasNoWritableAddress(t *testing.T) {
	html := RenderHTML("> - [ ] cita\n\n+ [ ] más\n")
	if strings.Contains(html, "data-todo-index") {
		t.Fatalf("unsupported source was addressable: %s", html)
	}
}

func TestProseSoftWrapsButKeepsExplicitBreaks(t *testing.T) {
	if strings.Contains(RenderHTML("primera línea\nsegunda línea\n"), "<br") {
		t.Fatal("soft-wrap became a premature hard break")
	}
	if !strings.Contains(RenderHTML("primera línea  \nsegunda línea\n"), "<br") {
		t.Fatal("explicit Markdown break was lost")
	}
}
