package bubble

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// Integraciones: otro sistema deja trabajo aquí de forma determinística.
//
// cuby.work es el primero: sus propuestas de tarea llegan como ideas al inbox de
// un equipo, y una tarea que se vuelve formal asegura su proyecto, su burbuja y
// su hilo. Del otro lado, los webhooks (webhooks.go) le avisan cuándo terminó.
//
// Quien llama es una PERSONA: la cuenta con la que la integración escribe aquí
// («cuby.work»), con su token de agente de siempre. Lo que crea queda firmado con
// ella, con las mismas reglas que cualquiera; quien pidió el trabajo allá viaja
// como `from`, igual que el remitente de un WhatsApp. Rechazado: un token de
// servicio que escriba a nombre de cada persona, porque aquí lo que se escribe
// queda firmado con el nombre de quien lo escribió.
//
// Todo es idempotente por (fuente, ref): reintentar después de una respuesta
// perdida encuentra lo que ya se creó en vez de abrir otro.

var sourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// IntegrationIdeaIn es una propuesta que llega de afuera.
type IntegrationIdeaIn struct {
	// Source nombra al sistema (`cuby`); Ref, la cosa allá (`task:123`).
	Source string `json:"source"`
	Ref    string `json:"ref"`
	// Team: id o nombre; vacío, el equipo por defecto de la cuenta.
	Team string `json:"team"`
	Note string `json:"note"`
	Body string `json:"body"`
	// From: quién la pidió allá, como se lee aquí.
	From string `json:"from"`
}

func checkSource(source, ref string) error {
	if !sourceName.MatchString(source) {
		return fmt.Errorf("`source` es el nombre del sistema: minúsculas, dígitos y guiones (p. ej. «cuby»)")
	}
	if strings.TrimSpace(ref) == "" || len(ref) > 200 {
		return fmt.Errorf("`ref` dice qué es la cosa allá, hasta 200 caracteres")
	}
	return nil
}

// IntegrateIdea crea la idea, o actualiza la que ya trajo esa misma ref.
func IntegrateIdea(app core.App, auth *core.Record, in IntegrationIdeaIn) (Idea, bool, error) {
	if !isPersonAuth(auth) {
		return Idea{}, false, ErrNotAPerson
	}
	if err := checkSource(in.Source, in.Ref); err != nil {
		return Idea{}, false, err
	}
	teamID, err := ResolveTeam(app, auth, in.Team)
	if err != nil {
		return Idea{}, false, err
	}
	if !InTeam(app, auth, teamID) {
		return Idea{}, false, fmt.Errorf("una idea va al inbox de un equipo del que la cuenta es parte")
	}
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return Idea{}, false, fmt.Errorf("una idea necesita un texto (`note`)")
	}
	r, err := app.FindFirstRecordByFilter("inbox_items",
		"team = {:t} && source = {:s} && source_ref = {:r}",
		dbx.Params{"t": teamID, "s": in.Source, "r": in.Ref})
	created := false
	if err != nil {
		col, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return Idea{}, false, err
		}
		r = core.NewRecord(col)
		r.Set("team", teamID)
		r.Set("captured_by", auth.Id)
		r.Set("source", in.Source)
		r.Set("source_ref", in.Ref)
		created = true
	}
	r.Set("note", note)
	r.Set("body", in.Body)
	r.Set("source_from", truncateText(strings.TrimSpace(in.From), 200))
	if err := app.Save(r); err != nil {
		return Idea{}, false, err
	}
	return ideaOf(app, r, true), created, nil
}

// IntegrationWorkIn es una tarea que se volvió formal allá: aquí se asegura su
// proyecto, su burbuja (opcional) y su hilo.
type IntegrationWorkIn struct {
	Source string `json:"source"`
	// Ref es la del hilo: la tarea allá.
	Ref     string `json:"ref"`
	Team    string `json:"team"`
	Project struct {
		Ref  string `json:"ref"`
		Name string `json:"name"`
		// Slug, si se da, adopta un proyecto que ya existe con esa dirección.
		Slug string `json:"slug"`
	} `json:"project"`
	Bubble *struct {
		Ref     string `json:"ref"`
		Name    string `json:"name"`
		Outcome string `json:"outcome"`
	} `json:"bubble,omitempty"`
	Thread struct {
		Name string `json:"name"`
		// Body: el documento inicial; sólo se escribe al crear el hilo.
		Body string `json:"body"`
		// Assignees: por correo; quien no ve el proyecto se suma, como al
		// asignar en la aplicación. Nil no toca a quién está asignado.
		Assignees *[]string `json:"assignees,omitempty"`
		Due       *string   `json:"due,omitempty"`
	} `json:"thread"`
	// Idea: la ref de la propuesta que llegó antes; se marca como trabajo.
	Idea string `json:"idea"`
}

// IntegrationWorkOut dice qué quedó y qué fue nuevo.
type IntegrationWorkOut struct {
	Project struct {
		ID      string `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		Created bool   `json:"created"`
	} `json:"project"`
	Bubble *struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Created bool   `json:"created"`
	} `json:"bubble,omitempty"`
	Thread struct {
		ID      string `json:"id"`
		Seq     int    `json:"seq"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Created bool   `json:"created"`
	} `json:"thread"`
	// Idea: el id de la idea que se marcó como trabajo, si la había.
	Idea string `json:"idea,omitempty"`
	// Unknown: correos que no son de nadie aquí; el hilo queda sin ellos.
	Unknown []string `json:"unknown,omitempty"`
}

// findRef: el id aquí de una cosa de allá, o vacío.
func findRef(app core.App, source, kind, ref string) string {
	r, err := app.FindFirstRecordByFilter("external_refs",
		"source = {:s} && kind = {:k} && ref = {:r}", dbx.Params{"s": source, "k": kind, "r": ref})
	if err != nil {
		return ""
	}
	return r.GetString("target")
}

func saveRef(app core.App, source, kind, ref, target, workspace string) error {
	col, err := app.FindCollectionByNameOrId("external_refs")
	if err != nil {
		return err
	}
	r := core.NewRecord(col)
	r.Set("source", source)
	r.Set("kind", kind)
	r.Set("ref", ref)
	r.Set("target", target)
	r.Set("workspace", workspace)
	return app.Save(r)
}

// IntegrateWork asegura proyecto, burbuja e hilo de una tarea de afuera.
func IntegrateWork(app core.App, auth *core.Record, t *tree.Tree, base string, in IntegrationWorkIn) (IntegrationWorkOut, error) {
	var out IntegrationWorkOut
	if !isPersonAuth(auth) {
		return out, ErrNotAPerson
	}
	if err := checkSource(in.Source, in.Ref); err != nil {
		return out, err
	}
	if strings.TrimSpace(in.Project.Ref) == "" && strings.TrimSpace(in.Project.Slug) == "" {
		return out, fmt.Errorf("el proyecto necesita una `ref` o un `slug`")
	}
	teamID, err := ResolveTeam(app, auth, in.Team)
	if err != nil {
		return out, err
	}

	// El proyecto: el de esa ref; si no, el de ese slug; si no, uno nuevo.
	var ws *core.Record
	var repo string
	if in.Project.Ref != "" {
		if id := findRef(app, in.Source, "project", in.Project.Ref); id != "" {
			if ws, repo, err = WorkspaceFor(app, auth, id); err != nil {
				return out, err
			}
		}
	}
	if ws == nil && in.Project.Slug != "" {
		if found, r, err := WorkspaceFor(app, auth, in.Project.Slug); err == nil {
			ws, repo = found, r
		}
	}
	if ws == nil {
		name := strings.TrimSpace(in.Project.Name)
		if name == "" {
			return out, fmt.Errorf("para crear el proyecto hace falta su `name`")
		}
		created, err := CreateWorkspace(app, auth, name, in.Project.Slug, teamID)
		if err != nil {
			return out, err
		}
		if ws, repo, err = WorkspaceFor(app, auth, created.Id); err != nil {
			return out, err
		}
		out.Project.Created = true
	}
	if ws.GetString("team") != teamID {
		return out, fmt.Errorf("el proyecto %q es de otro equipo", ws.GetString("slug"))
	}
	if in.Project.Ref != "" && findRef(app, in.Source, "project", in.Project.Ref) == "" {
		if err := saveRef(app, in.Source, "project", in.Project.Ref, ws.Id, ws.Id); err != nil {
			return out, err
		}
	}
	out.Project.ID, out.Project.Slug, out.Project.Name = ws.Id, ws.GetString("slug"), ws.GetString("name")

	// La burbuja, si se pidió: la de esa ref, o una nueva.
	bubbleID := ""
	if in.Bubble != nil {
		b := in.Bubble
		if strings.TrimSpace(b.Ref) == "" {
			return out, fmt.Errorf("la burbuja necesita una `ref`")
		}
		out.Bubble = &struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Created bool   `json:"created"`
		}{}
		if id := findRef(app, in.Source, "bubble", b.Ref); id != "" {
			if r, err := app.FindRecordById("bubbles", id); err == nil && r.GetString("workspace") == ws.Id {
				bubbleID = r.Id
				out.Bubble.Name = r.GetString("name")
			}
		}
		if bubbleID == "" {
			r, err := CreateBubble(app, auth, ws, b.Name, b.Outcome)
			if err != nil {
				return out, err
			}
			if err := saveRef(app, in.Source, "bubble", b.Ref, r.Id, ws.Id); err != nil {
				return out, err
			}
			bubbleID = r.Id
			out.Bubble.Name = r.GetString("name")
			out.Bubble.Created = true
		}
		out.Bubble.ID = bubbleID
	}

	// El hilo: el de esa ref, o uno nuevo con su documento inicial.
	var th *core.Record
	if id := findRef(app, in.Source, "thread", in.Ref); id != "" {
		if r, _, _, err := ThreadFor(app, auth, id); err == nil {
			th = r
		}
	}
	if th == nil {
		th, err = CreateThread(app, auth, ws, in.Thread.Name, bubbleID)
		if err != nil {
			return out, err
		}
		if err := saveRef(app, in.Source, "thread", in.Ref, th.Id, ws.Id); err != nil {
			return out, err
		}
		if body := in.Thread.Body; strings.TrimSpace(body) != "" {
			_, err := Apply(app, auth, t, ws, repo, th.GetString("doc_path"), th.GetString("name"),
				Patch{Base: tree.Hash(""), Content: &body, Message: "desde " + in.Source})
			if err != nil {
				return out, err
			}
		}
		out.Thread.Created = true
	}

	// Asignados y plazo: se ponen cada vez que vienen, así que repetir la
	// llamada deja lo mismo.
	edit := ThreadEdit{Due: in.Thread.Due}
	if in.Thread.Assignees != nil {
		ids := []string{}
		for _, email := range *in.Thread.Assignees {
			email = strings.TrimSpace(strings.ToLower(email))
			if email == "" {
				continue
			}
			u, err := app.FindAuthRecordByEmail("users", email)
			if err != nil || !u.GetDateTime("deleted_at").IsZero() {
				out.Unknown = append(out.Unknown, email)
				continue
			}
			ids = append(ids, u.Id)
		}
		edit.Assignees = &ids
	}
	if edit.Assignees != nil || edit.Due != nil {
		if th, err = SetThread(app, auth, th.Id, edit); err != nil {
			return out, err
		}
	}
	out.Thread.ID, out.Thread.Seq, out.Thread.Name = th.Id, th.GetInt("seq"), th.GetString("name")
	out.Thread.URL = strings.TrimRight(base, "/") + fmt.Sprintf("/w/%s/t/%d", ws.GetString("slug"), th.GetInt("seq"))

	// La propuesta que llegó antes: ahora apunta a su hilo.
	if in.Idea != "" {
		idea, err := app.FindFirstRecordByFilter("inbox_items",
			"team = {:t} && source = {:s} && source_ref = {:r}",
			dbx.Params{"t": teamID, "s": in.Source, "r": in.Idea})
		if err == nil {
			if _, err := IdeaDecide(app, auth, idea.Id, th.Id, ""); err != nil {
				return out, err
			}
			out.Idea = idea.Id
		}
	}
	return out, nil
}

func truncateText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func registerIntegrations(app core.App, t *tree.Tree) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		fail := func(e *core.RequestEvent, err error) error {
			if errors.Is(err, ErrDenied) {
				return e.NotFoundError("no existe o no lo ves", err)
			}
			return e.BadRequestError(err.Error(), err)
		}
		se.Router.POST("/api/integrations/ideas", func(e *core.RequestEvent) error {
			var in IntegrationIdeaIn
			if err := e.BindBody(&in); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			idea, created, err := IntegrateIdea(e.App, e.Auth, in)
			if err != nil {
				return fail(e, err)
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			return e.JSON(status, map[string]any{"idea": idea, "created": created})
		}).Bind(apis.RequireAuth("users"))

		se.Router.POST("/api/integrations/work", func(e *core.RequestEvent) error {
			var in IntegrationWorkIn
			if err := e.BindBody(&in); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			out, err := IntegrateWork(e.App, e.Auth, t, BaseURL(e.App, e.Request), in)
			if err != nil {
				return fail(e, err)
			}
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}
