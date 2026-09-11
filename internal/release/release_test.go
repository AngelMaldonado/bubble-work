package release

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
		why             string
	}{
		{"v1.0.0", "v1.0.1", true, "un parche es más nuevo"},
		{"v1.0.0", "v1.1.0", true, "un menor también"},
		{"v1.9.0", "v2.0.0", true, "y un mayor"},
		{"v1.0.0", "v1.0.0", false, "la misma no es más nueva"},
		{"v1.2.0", "v1.1.9", false, "ni una anterior"},
		{"v1.10.0", "v1.9.0", false, "diez es más que nueve, no menos"},

		// Lo que evita el aviso permanente en la máquina de quien escribe.
		{"v1.0.0-12-gabc1234-dirty", "v1.0.0", false, "un describe no es una versión"},
		{"dev", "v1.0.0", false, "ni `dev`"},
		{"v1.0.0", "", false, "ni una respuesta vacía de github"},
		{"v1.0.0", "v0-plane-as-record", false, "ni un tag con nombre"},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, quería %v — %s", c.current, c.latest, got, c.want, c.why)
		}
	}
}

func TestSumFor(t *testing.T) {
	sums := "abc123  bubble-linux-amd64\ndef456  bubble-darwin-arm64\n"
	if got := sumFor(sums, "bubble-darwin-arm64"); got != "def456" {
		t.Errorf("sumFor = %q", got)
	}
	// Una plataforma que ese release no publicó: hay que poder DECIRLO, no
	// bajar el binario de otra arquitectura y descubrirlo al reiniciar.
	if got := sumFor(sums, "bubble-windows-amd64"); got != "" {
		t.Errorf("sumFor de algo que no está = %q, quería vacío", got)
	}
}

func TestAsset(t *testing.T) {
	if got := Asset("linux", "arm64"); got != "bubble-linux-arm64" {
		t.Errorf("Asset = %q", got)
	}
}
