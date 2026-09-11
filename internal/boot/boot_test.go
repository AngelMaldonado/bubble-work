package boot

import (
	"os"
	"path/filepath"
	"testing"
)

// La tabla que el arranque tiene que cumplir. Es la razón de que `Decide` esté
// separado de los procesos: lo que hay que poder afirmar es esto, no que
// os/exec funciona.
func TestDecide(t *testing.T) {
	cases := []struct {
		name               string
		cur, good, pending string
		healthy            bool
		code               int
		wantTag            string
		wantRollback       bool
	}{
		{"una actualización pedida se aplica",
			"v1.0.0", "v1.0.0", "v1.1.0", true, PendingExit, "v1.1.0", false},

		{"la nueva no contestó: se vuelve a la última buena",
			"v1.1.0", "v1.0.0", "", false, 1, "v1.0.0", true},

		{"...y da igual cómo se haya muerto",
			"v1.1.0", "v1.0.0", "", false, 0, "v1.0.0", true},

		{"sin versión anterior no hay a dónde volver: se reintenta",
			"v1.0.0", "v1.0.0", "", false, 1, "v1.0.0", false},

		{"servía y se cayó: se relanza la misma, no es una actualización fallida",
			"v1.1.0", "v1.1.0", "", true, 1, "v1.1.0", false},

		{"un código de actualización sin nada pendiente no inventa una versión",
			"v1.0.0", "v1.0.0", "", true, PendingExit, "v1.0.0", false},
	}
	for _, c := range cases {
		got := Decide(c.cur, c.good, c.pending, c.healthy, c.code)
		if got.Tag != c.wantTag || got.Rollback != c.wantRollback {
			t.Errorf("%s: Decide(%q,%q,%q,%v,%d) = {%q rollback:%v}, quería {%q rollback:%v}",
				c.name, c.cur, c.good, c.pending, c.healthy, c.code,
				got.Tag, got.Rollback, c.wantTag, c.wantRollback)
		}
		if got.Why == "" {
			t.Errorf("%s: sin explicación — un rollback que no se explica es un misterio en el log", c.name)
		}
	}
}

func TestSeedYPunteros(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bubble-falso")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := Store{Dir: filepath.Join(dir, "bin")}

	if err := s.Seed(exe, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.0.0" || s.LastGood() != "v1.0.0" {
		t.Fatalf("tras sembrar: current=%q last-good=%q", s.Current(), s.LastGood())
	}
	st, err := os.Stat(s.Binary("v1.0.0"))
	if err != nil {
		t.Fatalf("el binario no quedó instalado: %v", err)
	}
	if st.Mode()&0o111 == 0 {
		t.Error("quedó sin permiso de ejecución, que es la mitad de instalarlo")
	}

	// Sembrar otra vez no pisa lo que ya hay: una instancia que ya se actualizó
	// no debe volver a la versión de la imagen cada vez que reinicia.
	if err := s.SetCurrent("v1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Install(exe, "v1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(exe, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.1.0" {
		t.Errorf("sembrar pisó la versión vigente: %q", s.Current())
	}

	// Un puntero que apunta a un binario que ya no está se reinstala en vez de
	// dejar la instancia sin arrancar.
	if err := os.Remove(s.Binary("v1.1.0")); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(exe, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.0.0" {
		t.Errorf("con el binario ausente debía reinstalarse, y current quedó %q", s.Current())
	}
}

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:8090": "http://127.0.0.1:8090/api/version",
		// Lo que trae la imagen. Escuchar «en todas» no es una dirección a la
		// que se pueda llamar, y preguntarle a 0.0.0.0 falla en silencio — que
		// aquí significaría dar por muerta una versión que estaba viva.
		"0.0.0.0:8090": "http://127.0.0.1:8090/api/version",
		":8090":        "http://127.0.0.1:8090/api/version",
		"":             "http://127.0.0.1:8090/api/version",
	}
	for addr, want := range cases {
		if got := HealthURL(addr); got != want {
			t.Errorf("HealthURL(%q) = %q, quería %q", addr, got, want)
		}
	}
}
