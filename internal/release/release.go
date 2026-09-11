// Package release answers one question: ¿hay una versión más nueva que ésta?
//
// La respuesta vive aquí y no en el navegador por dos razones. Una instancia con
// veinte pestañas abiertas haría veinte preguntas a GitHub, que limita por IP y
// contestaría 403 justo cuando alguien mira. Y la comparación es del SERVIDOR:
// él sabe qué binario está corriendo, y el navegador sólo sabe lo que le
// dijeron.
package release

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Repo es de dónde se publican los binarios. Una constante y no una
// configuración: un actualizador que apunta a donde le digan es un actualizador
// al que se le puede decir que apunte a otro sitio.
const Repo = "AngelMaldonado/bubble-work"

// Semver de esta serie, y sólo eso. `v1.0.0-12-gabc1234-dirty` —lo que sale de
// `git describe` en una máquina de trabajo— NO es una versión publicada, y
// comparar contra ella produciría un aviso de actualización permanente en el
// escritorio de quien está escribiendo el código.
var semver = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

// Newer dice si `latest` es posterior a `current`.
//
// Falso ante cualquier duda: si una de las dos no es una versión de esta serie,
// no hay nada que comparar. Avisar de una actualización que no existe enseña a
// ignorar el aviso, y entonces el que sí importa tampoco se lee.
func Newer(current, latest string) bool {
	a, ok1 := parse(current)
	b, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	m := semver.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Cache guarda la última respuesta de GitHub durante un rato.
//
// `/api/version` es la comprobación de salud del contenedor: contesta en cada
// arranque y cada treinta segundos, y no puede depender de una llamada a
// internet. Así que esto NUNCA bloquea — devuelve lo que tenga y refresca por
// detrás. Una instancia sin salida a internet contesta lo mismo que una con
// ella: su propia versión, y ningún aviso.
type Cache struct {
	Every time.Duration
	HTTP  *http.Client

	mu      sync.Mutex
	tag     string
	checked time.Time
	asking  bool
}

// New hace una caché que pregunta como mucho cada `every`.
func New(every time.Duration) *Cache {
	return &Cache{
		Every: every,
		HTTP:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Latest devuelve el último tag conocido y cuándo se supo. Nunca espera a la
// red: si lo que tiene está viejo, dispara una consulta y contesta con lo que
// hay — que la primera vez es nada.
func (c *Cache) Latest() (tag string, checked time.Time) {
	c.mu.Lock()
	tag, checked = c.tag, c.checked
	stale := time.Since(c.checked) > c.Every
	if stale && !c.asking {
		c.asking = true
		go c.refresh()
	}
	c.mu.Unlock()
	return tag, checked
}

func (c *Cache) refresh() {
	tag, err := fetch(context.Background(), c.HTTP)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asking = false
	if err != nil {
		// El reloj se toca igual: una instancia sin red no debe reintentar en
		// cada petición de salud, que son dos por minuto.
		c.checked = time.Now()
		return
	}
	c.tag, c.checked = tag, time.Now()
}

func fetch(ctx context.Context, cl *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", errStatus(res.StatusCode)
	}
	var out struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Tag, nil
}

type errStatus int

func (e errStatus) Error() string { return "github respondió " + strconv.Itoa(int(e)) }
