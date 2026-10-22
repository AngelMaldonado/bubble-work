package bubble

import (
	"strings"
	"testing"
)

// Las instructions de serie caben en lo que Claude Code entrega, ya rellenadas
// con los valores más largos.
func TestInstructionsFit(t *testing.T) {
	if err := validateInstructions(mustPrompt("instructions")); err != nil {
		t.Fatal(err)
	}
	got := Instructions(nil, "operators", strings.Repeat("a", 16))
	if n := utf16Len(got); n > InstructionsLimit {
		t.Fatalf("instructions miden %d", n)
	}
	for _, want := range []string{"release " + strings.Repeat("a", 16), "(operators)", "bubble-work connect " + ConnectVersion(nil)} {
		if !strings.Contains(got, want) {
			t.Fatalf("faltan %q en:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("quedó un hueco sin llenar:\n%s", got)
	}
}

func TestValidateInstructionsRejectsLong(t *testing.T) {
	if validateInstructions(strings.Repeat("ñ", InstructionsLimit+1)) == nil {
		t.Fatal("una edición de más de 2 048 caracteres se aceptó")
	}
	// Cuenta como el cliente: un emoji son dos unidades UTF-16.
	if validateInstructions(strings.Repeat("🫧", InstructionsLimit/2+1)) == nil {
		t.Fatal("contó runas en vez de unidades UTF-16")
	}
}

// La sección que se instala lleva su versión, y la versión es la de la sección
// sin rellenar: estable y sin depender de sí misma.
func TestConnectSection(t *testing.T) {
	v := ConnectVersion(nil)
	if len(v) != 16 {
		t.Fatalf("versión %q", v)
	}
	sec := ConnectSection(nil)
	// Texto visible, no un comentario HTML: Claude Code quita los comentarios del
	// CLAUDE.md antes de dárselo al modelo, y una marca que el modelo no ve le
	// hace creer que su sección está vieja.
	if !strings.HasPrefix(sec, "## Bubble Work") || !strings.Contains(sec, "`bubble-work connect "+v+"`") || strings.Contains(sec, "<!--") {
		t.Fatalf("sección sin marca:\n%s", sec)
	}
	if strings.Contains(ConnectText(nil), "{{version}}") {
		t.Fatal("el prompt salió sin versión")
	}
	if !strings.Contains(ConnectText(nil), "{{token}}") {
		t.Fatal("el hueco del token es del navegador")
	}
	if ConnectVersion(nil) != v {
		t.Fatal("la versión no es estable")
	}
}

func TestSectionOf(t *testing.T) {
	if sectionOf("sin bloque") != "" {
		t.Fatal("inventó una sección")
	}
	if got := sectionOf("a\n```markdown\nX\nY\n```\nb"); got != "X\nY" {
		t.Fatalf("%q", got)
	}
}

func TestCleanVersion(t *testing.T) {
	for in, want := range map[string]string{
		"a3f1c2":                "a3f1c2",
		" v1.2-x_y ":            "v1.2-x_y",
		"<script>":              "",
		strings.Repeat("a", 33): "",
		"":                      "",
	} {
		if got := cleanVersion(in); got != want {
			t.Fatalf("cleanVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
