package bubble

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Las dos columnas laterales de la secuencia, paginadas.
//
// A la izquierda lo que todavía no está en la línea y sigue abierto, lo más
// reciente primero; a la derecha lo que ya se terminó. Las dos crecen sin
// techo, y son justo las que no caben en `/api/board`: aquella respuesta
// compone el departamento ENTERO en un instante para poder clasificarlo por
// calor, y es lo correcto para un tablero que se lee de un vistazo. Una lista
// de todo lo que se ha terminado desde que existe el departamento no se lee de
// un vistazo — se recorre—, así que se pide por páginas.
//
// Lo que NO se manda aquí es el calor. Una fila de estas columnas ya dice lo
// único que hace falta saber de ella —está fuera de la línea, o está hecha— y
// calcular la banda de cada una costaría recorrer sus eventos para contestar
// una pregunta que nadie hace en esta pantalla.

// QueueCard es una fila de una de las dos columnas: lo justo para dibujar la
// misma tarjeta que el kanban.
type QueueCard struct {
	ID string `json:"id"`
	// El #N del hilo dentro de su proyecto. Un módulo no tiene.
	Seq       int    `json:"seq,omitempty"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	// La burbuja de un hilo. En la cola de módulos, vacío: el módulo ES la
	// burbuja.
	Bubble   string `json:"bubble,omitempty"`
	Priority string `json:"priority,omitempty"`
	Due      string `json:"due_date,omitempty"`
	// Cuándo nació, que es por lo que se ordenan estas columnas.
	Created string `json:"created"`
	// Quién la tiene: los asignados de un hilo, los responsables de un módulo.
	People []string `json:"people,omitempty"`
	// Su paso en la línea, para saber si vuelve a un sitio o entra por primera
	// vez. Cero en la columna de la izquierda, por definición.
	Sequence int `json:"sequence,omitempty"`
}

// QueuePage es una página y si hay más detrás.
type QueuePage struct {
	Items []QueueCard `json:"items"`
	Page  int         `json:"page"`
	// `more` y no un total: contar todo lo terminado de un departamento para
	// decidir si pintar un botón es una consulta cara por una respuesta que se
	// contesta pidiendo un elemento de más.
	More bool `json:"more"`
}

const (
	queuePerPage = 40
	queueMaxPage = 200
)

func registerQueue(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/queue", func(e *core.RequestEvent) error {
			ws, err := reachableWorkspaces(e)
			if err != nil {
				return err
			}
			q := e.Request.URL.Query()
			kind := q.Get("kind")
			if kind != "threads" && kind != "bubbles" {
				return e.BadRequestError("kind es `threads` o `bubbles`", nil)
			}
			state := q.Get("state")
			if state != "open" && state != "done" {
				return e.BadRequestError("state es `open` o `done`", nil)
			}
			page := atoiOr(q.Get("page"), 1)
			if page < 1 {
				page = 1
			}
			if page > queueMaxPage {
				return e.BadRequestError("esa página está demasiado lejos", nil)
			}
			per := atoiOr(q.Get("perPage"), queuePerPage)
			if per < 1 || per > queuePerPage {
				per = queuePerPage
			}

			// Sin workspaces alcanzables la respuesta es una página vacía, no un
			// error: alguien que no está en ningún proyecto tiene una cola vacía,
			// y eso es una respuesta.
			if len(ws) == 0 {
				return e.JSON(http.StatusOK, QueuePage{Items: []QueueCard{}, Page: page})
			}
			where := []string{reachFilter(ws)}
			switch {
			case kind == "threads" && state == "open":
				// «Fuera de la línea» y «sin terminar». Lo terminado se pregunta
				// por el ESTADO y no por `completed_at`: esa columna existe en el
				// esquema y no la escribe nadie, así que filtrar por ella habría
				// dado por abierto todo lo cerrado.
				where = append(where,
					"(sequence = 0 || sequence = null)",
					"(state = '' || state.group != 'completed')")
			case kind == "threads":
				// Lo hecho que falta REVISAR, no el historial entero: revisado
				// sale de aquí. Es lo que hace que esta columna sea una cola de
				// trabajo —se vacía— y no una lista que sólo crece.
				where = append(where, "state.group = 'completed'", "reviewed_at = ''")
			case kind == "bubbles" && state == "open":
				where = append(where, "(sequence = 0 || sequence = null)", "closed_at = ''")
			default:
				where = append(where, "closed_at != ''", "reviewed_at = ''")
			}

			// Uno de más: si vuelve, hay otra página. Contar el total sería una
			// consulta entera para pintar un botón.
			rows, err := e.App.FindRecordsByFilter(
				kind, strings.Join(where, " && "), "-created", per+1, (page-1)*per, dbx.Params{},
			)
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			more := len(rows) > per
			if more {
				rows = rows[:per]
			}

			out := make([]QueueCard, 0, len(rows))
			for _, r := range rows {
				c := QueueCard{
					ID: r.Id, Workspace: r.GetString("workspace"),
					Name:     r.GetString("name"),
					Priority: r.GetString("priority"),
					Due:      day(r.GetDateTime("due_date")),
					Created:  r.GetDateTime("created").String(),
					Sequence: r.GetInt("sequence"),
				}
				if kind == "threads" {
					c.Seq = r.GetInt("seq")
					c.Bubble = r.GetString("bubble")
					c.People = r.GetStringSlice("assignees")
				} else {
					c.People = r.GetStringSlice("owners")
				}
				out = append(out, c)
			}
			return e.JSON(http.StatusOK, QueuePage{Items: out, Page: page, More: more})
		}).Bind(apis.RequireAuth())

		return se.Next()
	})
}

// reachFilter limita una consulta a los workspaces de quien pregunta. La misma
// frontera que el board, escrita como filtro: sin esto, paginar sería una
// puerta para leer el trabajo de un proyecto en el que no se está.
func reachFilter(ws []*core.Record) string {
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		parts = append(parts, fmt.Sprintf("workspace = '%s'", w.Id))
	}
	return "(" + strings.Join(parts, " || ") + ")"
}
