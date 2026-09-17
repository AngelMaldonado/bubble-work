// Package boot es el arranque que puede deshacer una actualización.
//
// El binario que se instala —el de la imagen, o el que puso una persona— NO
// sirve nada: resuelve cuál es la versión vigente, la lanza como hijo y la
// vigila. Si esa versión no llega a contestar, vuelve a la última que sí lo hizo
// y la relanza. El que deshace nunca es el que se actualizó, que es la única
// forma de que quede alguien para deshacerlo.
//
//	<dir>/versions/v1.0.0    los binarios, uno por versión
//	<dir>/current            qué versión se está sirviendo
//	<dir>/last-good          la última que llegó a contestar de verdad
//	<dir>/next               lo que el servidor dejó pedido al actualizarse
//	<dir>/failed             la última versión que no llegó a contestar
//
// Los binarios viven en el DIRECTORIO DE DATOS y no en la imagen a propósito.
// En la imagen, un `docker compose up` los borraría y la instancia «volvería» a
// la versión de la imagen sin que nadie entienda por qué. El volumen es el
// estado; la imagen es sólo el arranque.
//
// Salvo cuando la imagen trae una versión MÁS NUEVA que la vigente: entonces es
// una actualización que llega por el otro camino —un orquestador que cambia la
// imagen, como Coolify— y se adopta igual que la del botón, con la vigente como
// red. Una imagen más vieja nunca pisa nada: quien actualizó con el botón no
// retrocede porque alguien reinició el contenedor.
package boot

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/release"
)

const (
	// PendingExit es cómo el servidor pide que lo relancen con otra versión: se
	// va con este código y el arranque lee `next`. Un código de salida y no una
	// señal ni un socket, porque es lo único que un hijo puede decirle a su
	// padre sin inventar un canal.
	PendingExit = 75 // EX_TEMPFAIL

	// ChildEnv marca al hijo. Sin esto, el servidor volvería a arrancarse a sí
	// mismo y la recursión sólo se notaría al quedarse sin procesos.
	ChildEnv = "BUBBLE_CHILD"

	current  = "current"
	lastGood = "last-good"
	next     = "next"
	failed   = "failed"
	versions = "versions"
)

// Store es el directorio donde viven las versiones y los punteros.
type Store struct{ Dir string }

// Dir devuelve dónde guardar los binarios: `BUBBLE_BIN_DIR`, o `bin/` al lado
// del directorio de datos — que es el sitio que ya se respalda.
func Dir(data string) string {
	if d := os.Getenv("BUBBLE_BIN_DIR"); d != "" {
		return d
	}
	if data == "" {
		data = "pb_data"
	}
	return filepath.Join(filepath.Dir(data), "bin")
}

func (s Store) path(name string) string  { return filepath.Join(s.Dir, name) }
func (s Store) Binary(tag string) string { return filepath.Join(s.Dir, versions, tag) }

func (s Store) read(name string) string {
	b, err := os.ReadFile(s.path(name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// write deja el puntero de una pieza, ATÓMICAMENTE: se escribe al lado y se
// renombra. Un `current` a medias es una instancia que no arranca, y ese es el
// único fallo del que este paquete no puede recuperarse.
func (s Store) write(name, tag string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp := s.path(name + ".tmp")
	if err := os.WriteFile(tmp, []byte(tag+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(name))
}

func (s Store) Current() string  { return s.read(current) }
func (s Store) LastGood() string { return s.read(lastGood) }
func (s Store) Next() string     { return s.read(next) }

func (s Store) SetCurrent(tag string) error  { return s.write(current, tag) }
func (s Store) SetLastGood(tag string) error { return s.write(lastGood, tag) }
func (s Store) SetNext(tag string) error     { return s.write(next, tag) }
func (s Store) ClearNext() error             { return os.Remove(s.path(next)) }
func (s Store) Failed() string               { return s.read(failed) }
func (s Store) SetFailed(tag string) error   { return s.write(failed, tag) }

// Seed deja instalada la versión que trae este binario, si no había ninguna o
// si la que trae es más nueva que la vigente.
//
// Es lo que hace que una instancia nueva arranque sin descargar nada: la
// primera versión vigente es, literalmente, una copia del ejecutable que la
// imagen trae dentro.
//
// Con una versión vigente, sólo la reemplaza una MÁS NUEVA, y sin tocar
// `last-good`: si la de la imagen no contesta, el arranque vuelve a la que
// servía. Y una que ya falló no se reintenta en cada reinicio del contenedor
// —cada intento es un minuto sin servicio—; la siguiente imagen, o el botón,
// sí.
func (s Store) Seed(exe, tag string) error {
	if tag == "" {
		return errors.New("un binario sin versión no se puede instalar: no habría cómo nombrarlo")
	}
	if cur := s.Current(); cur != "" {
		if _, err := os.Stat(s.Binary(cur)); err == nil {
			if !s.Adopts(tag) {
				return nil
			}
			if err := s.Install(exe, tag); err != nil {
				return err
			}
			return s.SetCurrent(tag)
		}
		// El puntero apunta a algo que no está. Pasa si alguien limpió el
		// volumen a mano; se reinstala en vez de morir.
	}
	if err := s.Install(exe, tag); err != nil {
		return err
	}
	if err := s.SetCurrent(tag); err != nil {
		return err
	}
	return s.SetLastGood(tag)
}

// Adopts dice si sembrar `tag` reemplazaría una versión vigente que funciona:
// es más nueva y no es la que ya falló. Quien arranca lo pregunta ANTES de
// sembrar, para copiar la base mientras nadie la está escribiendo.
func (s Store) Adopts(tag string) bool {
	cur := s.Current()
	if cur == "" {
		return false
	}
	if _, err := os.Stat(s.Binary(cur)); err != nil {
		return false
	}
	return release.Newer(cur, tag) && s.Failed() != tag
}

// Snapshot copia el directorio de datos en `<data>/backups/<name>`, con la
// forma de un respaldo de PocketBase —un zip del directorio sin `backups/`—
// para poder restaurarlo desde el dashboard como cualquier otro.
//
// Es la copia que `POST /api/update` hace con el servidor vivo, hecha aquí en
// frío: se llama antes de lanzar al hijo, así que SQLite no está a medio
// escribir.
func Snapshot(data, name string) error {
	dir := filepath.Join(data, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, name+".partial")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	walk := filepath.WalkDir(data, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(data, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel == "backups" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err := errors.Join(walk, zw.Close(), f.Close()); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

// Install copia un binario a su sitio con su nombre. Copiar y renombrar, nunca
// escribir encima del que se está ejecutando.
func (s Store) Install(src, tag string) error {
	if err := os.MkdirAll(filepath.Join(s.Dir, versions), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := s.Binary(tag) + ".partial"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Binary(tag))
}

// Decision es qué hacer cuando el hijo termina.
type Decision struct {
	Tag      string // la versión a lanzar ahora
	Rollback bool   // ...porque la anterior no sirvió
	Why      string // para el log, que es donde se entiende un rollback
}

// Decide contesta la única pregunta del arranque: el hijo se fue, ¿y ahora qué?
//
// Separado de los procesos para poder probarlo: lo que hay que poder afirmar es
// esta tabla, no que os/exec funciona.
func Decide(cur, good, pending string, healthy bool, code int) Decision {
	switch {
	case code == PendingExit && pending != "":
		return Decision{Tag: pending, Why: "actualizando a " + pending}

	case !healthy && good != "" && good != cur:
		// Nunca llegó a contestar. Es el caso para el que existe todo esto.
		return Decision{Tag: good, Rollback: true,
			Why: fmt.Sprintf("%s no llegó a contestar; volviendo a %s", cur, good)}

	case !healthy:
		// No hay a dónde volver: la que falla es la única que hay. Se reintenta,
		// porque la causa puede ser de fuera —un disco lleno, un puerto
		// ocupado— y rendirse dejaría la instancia muerta sin haberlo mirado.
		return Decision{Tag: cur, Why: "no llegó a contestar y no hay versión anterior; reintentando"}

	default:
		// Servía y se murió. Se relanza la misma: eso es un servicio, no una
		// actualización fallida.
		return Decision{Tag: cur, Why: "se detuvo; relanzando"}
	}
}
