// Package channels es por donde entra al inbox lo que no se escribe en la app.
//
// Un canal es un plugin COMPILADO: un paquete que se registra con `Register`
// desde su `init`, y que el binario incluye con un import. No es el paquete
// `plugin` de Go —sólo funciona en Linux y se rompe entre versiones— ni un
// servicio aparte: un canal que vive en otro proceso es otro proceso que
// desplegar y vigilar. Si algún día hace falta, un canal «webhook» genérico
// cabe en esta misma interfaz.
//
// El canal no sabe qué es el inbox, y el inbox no sabe qué es WhatsApp: un
// canal entrega mensajes normalizados (`Inbound`) a un `Sink`, y el núcleo los
// convierte en notas.
package channels

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Inbound es un mensaje que llegó por un canal, ya normalizado.
type Inbound struct {
	// Ref es el id del mensaje en su canal: lo que impide capturarlo dos veces.
	Ref string
	// From es quién lo mandó, dicho como lo dice el canal («Ana (+52 55…)»).
	From   string
	Text   string
	Images []Image
	At     time.Time
}

// Image es una imagen adjunta a un mensaje.
type Image struct {
	Name string
	Data []byte
	// Ref es el mensaje que la trajo, cuando no es el de la nota entera (una
	// foto de un álbum): borrar ese mensaje quita sólo esta imagen.
	Ref string
	// Text es el pie de esa imagen, para poder reescribirlo si se edita.
	Text string
}

// Sink recibe lo que llega. Devuelve `ErrDuplicate` si ese mensaje ya entró.
type Sink interface {
	Capture(ctx context.Context, kind string, in Inbound) error
	// Retract deshace lo capturado de un mensaje que se borró en su canal. Una
	// nota que ya se trió no se toca: sobre ella ya hay trabajo.
	Retract(ctx context.Context, kind string, ref string) (Outcome, error)
	// Revise reescribe el texto de un mensaje que se editó en su canal, con la
	// misma regla: una nota ya triada no se toca.
	Revise(ctx context.Context, kind string, ref string, text string) (Outcome, error)
}

// Outcome dice qué se hizo con un mensaje borrado o editado.
type Outcome string

const (
	OutcomeNone    Outcome = "none"    // no había nada de ese mensaje
	OutcomeDeleted Outcome = "deleted" // la nota se borró
	OutcomeTrimmed Outcome = "trimmed" // se quitó una imagen de la nota
	OutcomeUpdated Outcome = "updated" // se reescribió el texto de la nota
	OutcomeKept    Outcome = "kept"    // la nota ya se trió y se conserva
)

// Status es cómo está un canal ahora, para enseñarlo.
type Status struct {
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	// disabled · unlinked · pairing · connecting · connected · error
	State  string         `json:"state"`
	Detail string         `json:"detail,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
	// QR es el código a escanear mientras se vincula. No sale en JSON: se sirve
	// como imagen, para que el cliente no necesite una librería de QR.
	QR string `json:"-"`
}

// Store es lo que un canal puede leer y escribir de su fila en `channels`.
type Store interface {
	Config() map[string]any
	SaveConfig(cfg map[string]any) error
	// Logger escribe en los logs de la app (Dashboard → Logs), con el canal ya
	// dicho. Lo que pasa con un mensaje tiene que poder verse después.
	Logger() *slog.Logger
}

// Channel es un canal. `Start` se llama al encenderlo y al arrancar el servidor
// con él encendido; `Stop`, al apagarlo y al terminar.
type Channel interface {
	Kind() string
	Start(ctx context.Context, sink Sink) error
	Stop()
	Status() Status
	// Action es lo propio de cada canal —vincular, elegir un grupo—, con su
	// cuerpo en JSON. Las rutas son genéricas: `POST /api/channels/{kind}/{action}`.
	Action(ctx context.Context, action string, body json.RawMessage) (any, error)
}

// Factory construye un canal con el directorio donde puede guardar lo suyo
// (credenciales, sesión) y su configuración.
type Factory func(dataDir string, store Store) Channel

var (
	mu       sync.Mutex
	registry = map[string]Factory{}
)

// Register da de alta un tipo de canal. Se llama desde el `init` del plugin.
func Register(kind string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[kind] = f
}

// Kinds son los tipos compilados en este binario, en orden.
func Kinds() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func factory(kind string) (Factory, bool) {
	mu.Lock()
	defer mu.Unlock()
	f, ok := registry[kind]
	return f, ok
}
