package md

import (
	"reflect"
	"testing"
)

func TestTags(t *testing.T) {
	for _, tc := range []struct {
		name, doc string
		want      []string
	}{
		{"lista", "---\ntags: [Pagos, '#Móvil', pagos]\n---\n# Hola\n", []string{"pagos", "móvil"}},
		{"lista en bloque", "---\ntitle: x\ntags:\n  - Cliente/ACME\n  - ' infra  nube '\n---\nCuerpo", []string{"cliente/acme", "infra nube"}},
		{"texto con comas", "---\ntags: pagos, Móvil ,\n---\n", []string{"pagos", "móvil"}},
		{"números", "---\ntags: [2026, q4]\n---\n", []string{"2026", "q4"}},
		{"jerarquía con barras sobrantes", "---\ntags: ['/cliente//acme/']\n---\n", []string{"cliente/acme"}},
		{"NFC", "---\ntags: [\"A\u0301rea\", \"ÁREA\"]\n---\n", []string{"área"}},
		{"sin frontmatter", "# Título\ntags: [no]\n", []string{}},
		{"sin cierre", "---\ntags: [a]\nsigue", []string{}},
		{"YAML roto", "---\ntags: [a\n---\n", nil},
		{"sin tags", "---\ntitle: x\n---\n", []string{}},
		{"CRLF y BOM", "\ufeff---\r\ntags: [a]\r\n---\r\nx", []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Tags(tc.doc); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestTagsCapped(t *testing.T) {
	doc := "---\ntags: [a1,a2,a3,a4,a5,a6,a7,a8,a9,a10,a11,a12,a13,a14,a15,a16,a17,a18,a19,a20,a21]\n---\n"
	if n := len(Tags(doc)); n != maxTags {
		t.Fatalf("got %d tags, want %d", n, maxTags)
	}
	long := NormalizeTag("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if len([]rune(long)) != maxTagLen {
		t.Fatalf("tag of %d runes, want %d", len([]rune(long)), maxTagLen)
	}
}

func TestOnlyTagsChanged(t *testing.T) {
	body := "# Plan\n\n- [ ] paso\n"
	for _, tc := range []struct {
		name, prev, next string
		want             bool
	}{
		{"agregar frontmatter sólo con tags", body, "---\ntags: [a]\n---\n" + body, true},
		{"cambiar tags", "---\ntags: [a]\ntitle: X\n---\n" + body, "---\ntitle: X\ntags: [a, b]\n---\n" + body, true},
		{"quitar los tags", "---\ntags: [a]\n---\n" + body, body, true},
		{"tags y cuerpo", "---\ntags: [a]\n---\n" + body, "---\ntags: [b]\n---\n" + body + "más\n", false},
		{"otra clave del frontmatter", "---\ntags: [a]\ntitle: X\n---\n" + body, "---\ntags: [a]\ntitle: Y\n---\n" + body, false},
		{"sólo el cuerpo", body, body + "x", false},
		{"idéntico", body, body, false},
		{"YAML roto", "---\ntags: [a]\n---\n" + body, "---\ntags: [a\n---\n" + body, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OnlyTagsChanged(tc.prev, tc.next); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStripFrontmatter(t *testing.T) {
	if got := StripFrontmatter("---\ntags: [a]\n---\n# Hola\n"); got != "# Hola\n" {
		t.Fatalf("got %q", got)
	}
	if got := StripFrontmatter("# Sin bloque\n---\n"); got != "# Sin bloque\n---\n" {
		t.Fatalf("got %q", got)
	}
}
