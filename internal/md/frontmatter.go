package md

import (
	"reflect"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Frontmatter: el bloque YAML entre `---` al principio de un documento.
//
// Lo que Bubble lee de él son los TAGS (`tags: [pagos, cliente/acme]` o
// `tags: pagos, móvil`): la fuente de la clasificación de un documento, que
// escriben la persona y su IA (diseño del hilo #3, §6). El resto del bloque se
// conserva tal cual y no tiene autoridad: no cambia permisos, asignación ni
// fechas, y nada de él se ejecuta.
//
// El YAML lo lee un parser mantenido, no uno casero. Un bloque que no es YAML
// válido no rompe la escritura: simplemente no da tags.

const (
	maxTags   = 20
	maxTagLen = 60
)

// SplitFrontmatter separa el bloque del cuerpo. `ok` es falso si el documento
// no empieza con `---`; entonces `body` es el documento entero.
func SplitFrontmatter(content string) (front, body string, ok bool) {
	s := strings.TrimPrefix(content, "\ufeff")
	first, rest, found := strings.Cut(s, "\n")
	if !found || strings.TrimRight(first, " \t\r") != "---" {
		return "", content, false
	}
	pos := 0
	for pos <= len(rest) {
		line, after, more := strings.Cut(rest[pos:], "\n")
		if t := strings.TrimRight(line, " \t\r"); t == "---" || t == "..." {
			if !more {
				return rest[:pos], "", true
			}
			return rest[:pos], after, true
		}
		if !more {
			break
		}
		pos += len(line) + 1
	}
	return "", content, false // sin cierre: no es frontmatter
}

// StripFrontmatter devuelve el cuerpo, para dibujarlo sin el bloque YAML.
func StripFrontmatter(content string) string {
	_, body, _ := SplitFrontmatter(content)
	return body
}

func frontMap(content string) (map[string]any, bool) {
	front, _, ok := SplitFrontmatter(content)
	if !ok {
		return map[string]any{}, true
	}
	m := map[string]any{}
	if err := yaml.Unmarshal([]byte(front), &m); err != nil {
		return nil, false
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, true
}

// Tags son los tags del frontmatter, normalizados: sin `#`, sin distinguir
// mayúsculas, Unicode NFC, espacios colapsados, jerarquías con `/`
// (`cliente/acme`). Sin repetir, en el orden escrito: el primero es el que la
// persona puso primero, y la vista de blobs lo usa para ubicar el punto.
func Tags(content string) []string {
	m, ok := frontMap(content)
	if !ok {
		return nil
	}
	var raw []string
	switch v := m["tags"].(type) {
	case string:
		raw = strings.Split(v, ",")
	case []any:
		for _, x := range v {
			switch y := x.(type) {
			case string:
				raw = append(raw, y)
			case int, int64, float64, bool:
				raw = append(raw, yamlScalar(y))
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range raw {
		n := NormalizeTag(t)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		if len(out) == maxTags {
			break
		}
	}
	return out
}

func yamlScalar(v any) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

var fold = cases.Fold()

// NormalizeTag: la forma única de un tag. Vacío si no queda nada.
func NormalizeTag(t string) string {
	t = norm.NFC.String(strings.TrimSpace(t))
	t = strings.TrimLeft(t, "#")
	t = fold.String(strings.Join(strings.Fields(t), " "))
	parts := []string{}
	for _, p := range strings.Split(t, "/") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	t = strings.Join(parts, "/")
	if utf8.RuneCountInString(t) > maxTagLen {
		r := []rune(t)
		t = strings.TrimRight(string(r[:maxTagLen]), "/ ")
	}
	return t
}

// OnlyTagsChanged: ¿la diferencia entre dos versiones es SÓLO la clave `tags`
// del frontmatter? Entonces es clasificar, no producir: no calienta. El cuerpo
// tiene que ser idéntico y el resto del frontmatter, el mismo dato. Un
// frontmatter ilegible cuenta como cambio de contenido, por si acaso.
func OnlyTagsChanged(prev, next string) bool {
	if prev == next {
		return false
	}
	_, bodyA, _ := SplitFrontmatter(prev)
	_, bodyB, _ := SplitFrontmatter(next)
	if bodyA != bodyB {
		return false
	}
	a, okA := frontMap(prev)
	b, okB := frontMap(next)
	if !okA || !okB {
		return false
	}
	delete(a, "tags")
	delete(b, "tags")
	return reflect.DeepEqual(a, b)
}
