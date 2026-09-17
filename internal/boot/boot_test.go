package boot

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
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

func TestSeedAdoptaUnaImagenMasNueva(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bubble-falso")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := Store{Dir: filepath.Join(dir, "bin")}
	if err := s.Seed(exe, "v1.1.0"); err != nil {
		t.Fatal(err)
	}

	if s.Adopts("v1.1.0") || s.Adopts("v1.0.0") || s.Adopts("dev") {
		t.Error("sólo una versión más nueva se adopta")
	}
	if !s.Adopts("v1.2.0") {
		t.Error("una imagen más nueva debía adoptarse")
	}

	// Coolify cambió la imagen: la versión nueva pasa a ser la vigente, y la
	// anterior sigue siendo la red.
	if err := s.Seed(exe, "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.2.0" {
		t.Errorf("una imagen más nueva debía adoptarse, y current quedó %q", s.Current())
	}
	if s.LastGood() != "v1.1.0" {
		t.Errorf("adoptarla no puede declararla buena antes de contestar: last-good=%q", s.LastGood())
	}
	if _, err := os.Stat(s.Binary("v1.2.0")); err != nil {
		t.Errorf("el binario de la imagen no quedó instalado: %v", err)
	}

	// Una imagen más vieja, o la misma, no toca nada.
	for _, tag := range []string{"v1.0.0", "v1.2.0", "dev"} {
		if err := s.Seed(exe, tag); err != nil {
			t.Fatal(err)
		}
		if s.Current() != "v1.2.0" {
			t.Errorf("sembrar %s movió la vigente a %q", tag, s.Current())
		}
	}

	// La que ya falló no se reintenta al reiniciar con la misma imagen.
	if err := s.SetCurrent("v1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFailed("v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(exe, "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.1.0" {
		t.Errorf("reintentó una versión que ya había fallado: current=%q", s.Current())
	}
	// ...pero la siguiente imagen sí.
	if err := s.Seed(exe, "v1.3.0"); err != nil {
		t.Fatal(err)
	}
	if s.Current() != "v1.3.0" {
		t.Errorf("una fallida no debe bloquear la siguiente: current=%q", s.Current())
	}
}

func TestSnapshot(t *testing.T) {
	data := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(data, "data.db"), []byte("base"), 0o644))
	must(os.MkdirAll(filepath.Join(data, "storage", "x"), 0o755))
	must(os.WriteFile(filepath.Join(data, "storage", "x", "a.png"), []byte("png"), 0o644))
	must(os.MkdirAll(filepath.Join(data, "backups"), 0o755))
	must(os.WriteFile(filepath.Join(data, "backups", "vieja.zip"), []byte("zip"), 0o644))

	must(Snapshot(data, "antes-de-v1.2.0.zip"))

	zr, err := zip.OpenReader(filepath.Join(data, "backups", "antes-de-v1.2.0.zip"))
	must(err)
	defer zr.Close()
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	if !got["data.db"] || !got["storage/x/a.png"] {
		t.Errorf("la copia no trae la base y los archivos: %v", got)
	}
	for name := range got {
		if strings.HasPrefix(name, "backups/") {
			t.Errorf("la copia se metió a sí misma o a las anteriores: %s", name)
		}
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
