package bubble

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// Webhooks: Bubble avisa a otro sistema que algo pasó aquí.
//
// El primero es cuby.work, que quiere saber cuándo terminó el hilo de una tarea
// suya para proponer su término. A diferencia del webhook de los semáforos, aquí
// un aviso perdido no se corrige solo: no hay latido. Por eso cada aviso es una
// fila en `webhook_deliveries` (una por evento y destino) que se reintenta con
// espera creciente hasta que el receptor contesta 2xx, o se da por muerta.
//
// La fuente es la tabla `events`, que ya registra todo lo que calienta: un
// webhook no inventa otra cuenta de lo que pasó, la lee. Cada aviso va firmado
// como el de los semáforos (`X-Bubble-Signature: sha256=<hex>`, HMAC-SHA256 del
// cuerpo) y lleva el id del evento, que es la llave de idempotencia del receptor.

// webhookTypes: qué evento de aquí se avisa, con qué nombre de afuera.
var webhookTypes = map[string]string{
	EvThreadCompleted: "thread.completed",
}

// WebhookEventNames son los que un webhook puede escuchar.
func WebhookEventNames() []string {
	out := []string{}
	for _, v := range webhookTypes {
		out = append(out, v)
	}
	return out
}

const (
	webhookMaxAttempts = 12
	webhookBatch       = 50
)

// webhookBackoff: 1, 2, 4… minutos, hasta 6 horas entre intentos. Doce intentos
// cubren poco más de un día: lo que tarda en notarse un receptor caído.
func webhookBackoff(attempts int) time.Duration {
	d := time.Minute << uint(attempts)
	if d <= 0 || d > 6*time.Hour {
		return 6 * time.Hour
	}
	return d
}

type webhookSender struct {
	mu     sync.Mutex
	timer  *time.Timer
	run    sync.Mutex
	client *http.Client
}

var hooksOut = &webhookSender{client: &http.Client{Timeout: 10 * time.Second}}

func (w *webhookSender) soon(app core.App, t *tree.Tree) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(2*time.Second, func() { deliverDue(app, t) })
}

// enqueueWebhooks: un evento nuevo que algún webhook escucha se vuelve un aviso
// pendiente por destino.
func enqueueWebhooks(app core.App, ev *core.Record) bool {
	typ, ok := webhookTypes[ev.GetString("kind")]
	if !ok {
		return false
	}
	hooks, err := app.FindRecordsByFilter("webhooks", "active = true", "", 0, 0)
	if err != nil || len(hooks) == 0 {
		return false
	}
	col, err := app.FindCollectionByNameOrId("webhook_deliveries")
	if err != nil {
		return false
	}
	queued := false
	for _, h := range hooks {
		if !listens(h, typ) {
			continue
		}
		if skip := h.GetString("skip_actor"); skip != "" && skip == ev.GetString("actor") {
			continue
		}
		d := core.NewRecord(col)
		d.Set("webhook", h.Id)
		d.Set("event", ev.Id)
		d.Set("type", typ)
		d.Set("status", "pending")
		d.Set("next_at", time.Now().UTC())
		if err := app.Save(d); err != nil {
			app.Logger().Error("webhook: no se pudo encolar", "webhook", h.Id, "event", ev.Id, "err", err)
			continue
		}
		queued = true
	}
	return queued
}

func listens(h *core.Record, typ string) bool {
	var evs []string
	_ = h.UnmarshalJSONField("events", &evs)
	for _, e := range evs {
		if e == typ {
			return true
		}
	}
	return false
}

// deliverDue manda los avisos que ya tocan. Uno a la vez en todo el proceso:
// el cron y el «pronto» de un evento nuevo pueden coincidir.
func deliverDue(app core.App, t *tree.Tree) (sent, failed int) {
	if !hooksOut.run.TryLock() {
		return 0, 0
	}
	defer hooksOut.run.Unlock()
	due, err := app.FindRecordsByFilter("webhook_deliveries",
		"status = 'pending' && next_at <= {:now}", "next_at", webhookBatch, 0,
		dbx.Params{"now": time.Now().UTC().Format("2006-01-02 15:04:05.000Z")})
	if err != nil {
		return 0, 0
	}
	for _, d := range due {
		if deliverOne(app, t, d) {
			sent++
		} else {
			failed++
		}
	}
	return sent, failed
}

// deliverOne intenta un aviso y deja escrito cómo le fue.
func deliverOne(app core.App, t *tree.Tree, d *core.Record) bool {
	h, err := app.FindRecordById("webhooks", d.GetString("webhook"))
	if err != nil {
		return false
	}
	ev, err := app.FindRecordById("events", d.GetString("event"))
	if err != nil {
		d.Set("status", "dead")
		d.Set("last_error", "el evento ya no existe")
		_ = app.Save(d)
		return false
	}
	payload := webhookPayload(app, t, d.GetString("type"), ev)
	status, err := postSigned(h, d.GetString("type"), d.Id, payload)
	attempts := d.GetInt("attempts") + 1
	d.Set("attempts", attempts)
	ok := err == nil
	if ok {
		d.Set("status", "sent")
		d.Set("last_error", "")
	} else {
		d.Set("last_error", truncateText(err.Error(), 300))
		if attempts >= webhookMaxAttempts {
			d.Set("status", "dead")
		} else {
			d.Set("next_at", time.Now().UTC().Add(webhookBackoff(attempts-1)))
		}
	}
	if err := app.Save(d); err != nil {
		app.Logger().Error("webhook: no se pudo anotar el intento", "delivery", d.Id, "err", err)
	}
	h.Set("last_status", truncateText(status, 300))
	h.Set("last_at", time.Now().UTC())
	_ = app.Save(h)
	return ok
}

// postSigned manda un cuerpo firmado. Devuelve la línea de estado que se
// enseña en Ajustes, y un error si no fue 2xx.
func postSigned(h *core.Record, typ, delivery string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "no se pudo armar el aviso", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.GetString("url"), bytes.NewReader(body))
	if err != nil {
		return "URL inválida", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "bubble-work-webhook")
	req.Header.Set("X-Bubble-Event", typ)
	req.Header.Set("X-Bubble-Delivery", delivery)
	req.Header.Set("X-Bubble-Signature", Sign(h.GetString("secret"), body))
	res, err := hooksOut.client.Do(req)
	if err != nil {
		return "sin respuesta: " + err.Error(), err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		err := fmt.Errorf("el receptor contestó %d", res.StatusCode)
		return err.Error(), err
	}
	return fmt.Sprintf("%s entregado (%d)", typ, res.StatusCode), nil
}

// WebhookThread es el hilo, como lo lee el receptor.
type WebhookThread struct {
	ID      string            `json:"id"`
	Seq     int               `json:"seq"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Project map[string]string `json:"project"`
	Bubble  map[string]string `json:"bubble,omitempty"`
	// Refs: qué es este hilo en otros sistemas (lo que dejó /api/integrations).
	Refs []map[string]string `json:"refs"`
	// Checks: casillas marcadas y totales del documento.
	Checks map[string]int `json:"checks"`
	// Summary: el principio del documento, sin su frontmatter.
	Summary string `json:"summary"`
}

var (
	checkedBox = regexp.MustCompile(`(?m)^\s*[-*] \[[xX]\]`)
	anyBox     = regexp.MustCompile(`(?m)^\s*[-*] \[[ xX]\]`)
)

// webhookPayload arma el aviso de un evento. El id del evento es la llave de
// idempotencia: un reintento manda el mismo.
func webhookPayload(app core.App, t *tree.Tree, typ string, ev *core.Record) map[string]any {
	out := map[string]any{"id": ev.Id, "type": typ, "at": ev.GetString("at")}
	if actor, err := app.FindRecordById("users", ev.GetString("actor")); err == nil {
		out["actor"] = map[string]string{"id": actor.Id, "name": actor.GetString("name"), "email": actor.Email()}
	}
	if ev.GetString("target_type") != "thread" {
		return out
	}
	th, err := app.FindRecordById("threads", ev.GetString("target"))
	if err != nil {
		return out
	}
	ws, err := app.FindRecordById("workspaces", th.GetString("workspace"))
	if err != nil {
		return out
	}
	wt := WebhookThread{
		ID: th.Id, Seq: th.GetInt("seq"), Name: th.GetString("name"),
		URL:     strings.TrimRight(app.Settings().Meta.AppURL, "/") + fmt.Sprintf("/w/%s/t/%d", ws.GetString("slug"), th.GetInt("seq")),
		Project: map[string]string{"id": ws.Id, "slug": ws.GetString("slug"), "name": ws.GetString("name")},
		Refs:    []map[string]string{},
		Checks:  map[string]int{"done": 0, "total": 0},
	}
	if b, err := app.FindRecordById("bubbles", th.GetString("bubble")); err == nil {
		wt.Bubble = map[string]string{"id": b.Id, "name": b.GetString("name")}
	}
	if refs, err := app.FindRecordsByFilter("external_refs", "kind = 'thread' && target = {:t}", "", 0, 0,
		dbx.Params{"t": th.Id}); err == nil {
		for _, r := range refs {
			wt.Refs = append(wt.Refs, map[string]string{"source": r.GetString("source"), "ref": r.GetString("ref")})
		}
	}
	if t != nil {
		if doc, _, err := t.Read(ws.GetString("repo_path"), th.GetString("doc_path")); err == nil {
			wt.Checks["done"] = len(checkedBox.FindAllString(doc, -1))
			wt.Checks["total"] = len(anyBox.FindAllString(doc, -1))
			wt.Summary = truncateText(strings.TrimSpace(stripFrontmatter(doc)), 1500)
		}
	}
	out["thread"] = wt
	return out
}

func stripFrontmatter(doc string) string {
	if !strings.HasPrefix(doc, "---\n") {
		return doc
	}
	if end := strings.Index(doc[4:], "\n---"); end >= 0 {
		rest := doc[4+end+4:]
		return strings.TrimPrefix(rest, "\n")
	}
	return doc
}

func webhookJSON(app core.App, h *core.Record) map[string]any {
	var evs []string
	_ = h.UnmarshalJSONField("events", &evs)
	if evs == nil {
		evs = []string{}
	}
	pending, _ := app.CountRecords("webhook_deliveries",
		dbx.HashExp{"webhook": h.Id, "status": "pending"})
	dead, _ := app.CountRecords("webhook_deliveries",
		dbx.HashExp{"webhook": h.Id, "status": "dead"})
	skip := ""
	if u, err := app.FindRecordById("users", h.GetString("skip_actor")); err == nil {
		skip = u.Email()
	}
	return map[string]any{
		"id": h.Id, "name": h.GetString("name"), "url": h.GetString("url"), "events": evs,
		"active": h.GetBool("active"), "skip_actor": skip, "has_secret": h.GetString("secret") != "",
		"last_status": h.GetString("last_status"), "last_at": h.GetString("last_at"),
		"pending": pending, "dead": dead,
	}
}

// webhookIn: lo que se puede decir de un webhook. Punteros: «no lo mencioné» y
// «déjalo vacío» son peticiones distintas.
type webhookIn struct {
	Name   *string   `json:"name"`
	URL    *string   `json:"url"`
	Events *[]string `json:"events"`
	Active *bool     `json:"active"`
	// SkipActor: el correo de la cuenta con la que la integración escribe aquí.
	SkipActor *string `json:"skip_actor"`
	Rotate    bool    `json:"rotate"`
}

func applyWebhook(app core.App, h *core.Record, in webhookIn) error {
	if in.Name != nil {
		h.Set("name", strings.TrimSpace(*in.Name))
	}
	if in.URL != nil {
		h.Set("url", strings.TrimSpace(*in.URL))
	}
	if in.Events != nil {
		known := map[string]bool{}
		for _, v := range webhookTypes {
			known[v] = true
		}
		for _, e := range *in.Events {
			if !known[e] {
				return fmt.Errorf("no hay un evento %q; los que hay: %s", e, strings.Join(WebhookEventNames(), ", "))
			}
		}
		h.Set("events", *in.Events)
	}
	if in.Active != nil {
		h.Set("active", *in.Active)
	}
	if in.SkipActor != nil {
		h.Set("skip_actor", "")
		if email := strings.TrimSpace(*in.SkipActor); email != "" {
			u, err := app.FindAuthRecordByEmail("users", email)
			if err != nil {
				return fmt.Errorf("no hay nadie con el correo %q", email)
			}
			h.Set("skip_actor", u.Id)
		}
	}
	return nil
}

func registerWebhooks(app core.App, t *tree.Tree) {
	app.OnRecordAfterCreateSuccess("events").BindFunc(func(e *core.RecordEvent) error {
		if enqueueWebhooks(e.App, e.Record) {
			hooksOut.soon(e.App, t)
		}
		return e.Next()
	})
	// Lo que falló espera su turno: el cron lo vuelve a intentar.
	app.Cron().MustAdd("bubbleWebhooks", "* * * * *", func() { deliverDue(app, t) })

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		lead := func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("los webhooks son del lead global", nil)
			}
			return nil
		}
		se.Router.GET("/api/webhooks", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			rows, err := e.App.FindRecordsByFilter("webhooks", "", "created", 0, 0)
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			out := []map[string]any{}
			for _, r := range rows {
				out = append(out, webhookJSON(e.App, r))
			}
			return e.JSON(http.StatusOK, map[string]any{"webhooks": out, "events": WebhookEventNames()})
		}).Bind(apis.RequireAuth("users"))

		// Crear: el secreto nace aquí y es la ÚNICA vez que sale del servidor.
		se.Router.POST("/api/webhooks", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			var in webhookIn
			if err := e.BindBody(&in); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			col, err := e.App.FindCollectionByNameOrId("webhooks")
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			h := core.NewRecord(col)
			h.Set("active", true)
			if err := applyWebhook(e.App, h, in); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			secret := newSecret()
			h.Set("secret", secret)
			if err := e.App.Save(h); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			out := webhookJSON(e.App, h)
			out["secret"] = secret
			return e.JSON(http.StatusCreated, out)
		}).Bind(apis.RequireAuth("users"))

		se.Router.POST("/api/webhooks/{id}", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			h, err := e.App.FindRecordById("webhooks", e.Request.PathValue("id"))
			if err != nil {
				return e.NotFoundError("no existe", err)
			}
			var in webhookIn
			if err := e.BindBody(&in); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			if err := applyWebhook(e.App, h, in); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			fresh := ""
			if in.Rotate {
				fresh = newSecret()
				h.Set("secret", fresh)
			}
			if err := e.App.Save(h); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			out := webhookJSON(e.App, h)
			if fresh != "" {
				out["secret"] = fresh
			}
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth("users"))

		se.Router.DELETE("/api/webhooks/{id}", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			h, err := e.App.FindRecordById("webhooks", e.Request.PathValue("id"))
			if err != nil {
				return e.NotFoundError("no existe", err)
			}
			if err := e.App.Delete(h); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.NoContent(http.StatusNoContent)
		}).Bind(apis.RequireAuth("users"))

		// Un `ping` firmado ahora, para probar el receptor; no pasa por la cola.
		se.Router.POST("/api/webhooks/{id}/test", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			h, err := e.App.FindRecordById("webhooks", e.Request.PathValue("id"))
			if err != nil {
				return e.NotFoundError("no existe", err)
			}
			status, perr := postSigned(h, "ping", "ping", map[string]any{
				"id": "ping", "type": "ping", "at": time.Now().UTC().Format(time.RFC3339),
			})
			h.Set("last_status", truncateText(status, 300))
			h.Set("last_at", time.Now().UTC())
			_ = e.App.Save(h)
			out := webhookJSON(e.App, h)
			out["ok"] = perr == nil
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}
