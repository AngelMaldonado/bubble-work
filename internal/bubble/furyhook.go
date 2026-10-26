package bubble

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/AngelMaldonado/bubble-work/internal/fury"
)

// El webhook de los semáforos (hilo #6, historia 3).
//
// Bubble AVISA (push) a una sola URL, que configura el lead global; del otro
// lado, el receptor del hardware reparte a cada escritorio. Un aviso por
// persona cuyo nivel cambió, y un LATIDO cada 5 minutos con el nivel de todas
// las personas activas: un semáforo que se reinicia se pone al día en el
// siguiente latido, y el firmware apaga la luz si deja de oír latidos (el verde
// nunca miente). Sin reintentos: el latido corrige.
//
// La luz de una persona es la furia de lo suyo: hilos asignados y burbujas de
// las que responde (como su cabeza en el dock). Con la furia apagada en el
// departamento, todas en Zen (verde). Fuera de horario la luz sigue diciendo
// el nivel: el horario es del firmware.
//
// Cada aviso va firmado: `X-Bubble-Signature: sha256=<hex>`, el HMAC-SHA256
// del cuerpo con el secreto, que se enseña una sola vez. Rechazados: MQTT y
// que el semáforo pregunte (decisión de Angel, push por webhook) y una URL por
// semáforo (Bubble tendría que alcanzar IPs de la oficina).

// FuryLight es lo que recibe el receptor por persona.
type FuryLight struct {
	Type    string   `json:"type"` // fury.changed | fury.heartbeat
	Person  string   `json:"person"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Level   int      `json:"level"`
	Mood    string   `json:"mood"` // Zen, Irritada…
	Color   string   `json:"color"`
	Blink   bool     `json:"blink"`
	Reasons []string `json:"reasons"`
	At      string   `json:"at"`
}

// furyLights calcula la luz de cada persona activa, con un solo cálculo de
// todos los tableros. Sin furia encendida, todas Zen.
func furyLights(app core.App, now time.Time) []FuryLight {
	people, err := app.FindRecordsByFilter("users", "deleted_at = ''", "email", 0, 0)
	if err != nil {
		return nil
	}
	best := map[string]*fury.Verdict{}
	type cause struct {
		level int
		text  string
	}
	why := map[string][]cause{}
	if rules := activeFuryRules(app); rules != nil {
		wss, _ := app.FindAllRecords("workspaces")
		take := func(ids []string, v *fury.Verdict, label string) {
			if v == nil {
				return
			}
			for _, id := range ids {
				if b := best[id]; b == nil || v.Level.Level > b.Level.Level {
					best[id] = v
				}
				if len(v.Reasons) > 0 {
					why[id] = append(why[id], cause{v.Level.Level, fmt.Sprintf("«%s» %s", label, v.Reasons[0].Text)})
				}
			}
		}
		for _, ws := range wss {
			b, err := boardFor(app, ws, rules)
			if err != nil {
				continue
			}
			for _, x := range b.Bubbles {
				take(x.Owners, x.Fury, x.Name)
			}
			for _, x := range b.Threads {
				take(x.Assignees, x.Fury, x.Name)
			}
		}
	}
	at := now.UTC().Format(time.RFC3339)
	out := make([]FuryLight, 0, len(people))
	for _, u := range people {
		lv := fury.Levels[fury.Zen]
		if v := best[u.Id]; v != nil {
			lv = v.Level
		}
		name := u.GetString("display_name")
		if name == "" {
			name = u.Email()
		}
		// La más grave primero: es la que el semáforo enseña si sólo cabe una.
		cs := why[u.Id]
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].level > cs[j].level })
		reasons := []string{}
		for _, c := range cs {
			if len(reasons) == 5 {
				break
			}
			reasons = append(reasons, c.text)
		}
		out = append(out, FuryLight{Person: u.Id, Email: u.Email(), Name: name, Level: lv.Level, Mood: lv.Name,
			Color: lv.Color, Blink: lv.Blink, Reasons: reasons, At: at})
	}
	return out
}

// furyHook lleva lo que el proceso recuerda entre envíos: el último nivel que
// se avisó de cada persona. En memoria: al arrancar, el primer cálculo cuenta
// como cambio para todos, que es lo mismo que un latido.
type furyHook struct {
	mu     sync.Mutex
	sent   map[string]int
	timer  *time.Timer
	client *http.Client
}

var furySent = &furyHook{sent: map[string]int{}, client: &http.Client{Timeout: 5 * time.Second}}

func hookRow(app core.App) *core.Record {
	r, err := app.FindFirstRecordByFilter("fury_hook", "id != ''")
	if err != nil {
		return nil
	}
	return r
}

// Sign es la firma de un cuerpo: lo que el receptor recalcula para fiarse.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// pushFury avisa: con `heartbeat`, a todos; si no, sólo a quien cambió.
// Devuelve cuántos avisos salieron y cuántos fallaron.
func pushFury(app core.App, heartbeat bool) (sent, failed int, status string) {
	row := hookRow(app)
	if row == nil || row.GetString("url") == "" || row.GetString("secret") == "" {
		return 0, 0, "sin URL configurada"
	}
	url, secret := row.GetString("url"), row.GetString("secret")
	lights := furyLights(app, time.Now())

	furySent.mu.Lock()
	var due []FuryLight
	for _, l := range lights {
		prev, seen := furySent.sent[l.Person]
		if heartbeat || !seen || prev != l.Level {
			l.Type = "fury.changed"
			if heartbeat && seen && prev == l.Level {
				l.Type = "fury.heartbeat"
			}
			due = append(due, l)
		}
	}
	furySent.mu.Unlock()

	last := ""
	for _, l := range due {
		body, _ := json.Marshal(l)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "bubble-work-fury")
		req.Header.Set("X-Bubble-Event", l.Type)
		req.Header.Set("X-Bubble-Signature", Sign(secret, body))
		res, err := furySent.client.Do(req)
		cancel()
		switch {
		case err != nil:
			failed++
			last = err.Error()
		case res.StatusCode >= 300:
			failed++
			last = fmt.Sprintf("el receptor contestó %d", res.StatusCode)
			res.Body.Close()
		default:
			res.Body.Close()
			sent++
			furySent.mu.Lock()
			furySent.sent[l.Person] = l.Level
			furySent.mu.Unlock()
		}
	}
	status = fmt.Sprintf("%d enviados", sent)
	if failed > 0 {
		status = fmt.Sprintf("%d enviados, %d fallaron: %s", sent, failed, last)
	}
	if len(due) > 0 || heartbeat {
		row.Set("last_status", status)
		row.Set("last_sent", sent)
		row.Set("last_at", time.Now().UTC())
		_ = app.Save(row)
	}
	return sent, failed, status
}

// soon agrupa una ráfaga de escrituras en un solo cálculo, 2 s después de la
// última: crear un hilo y su documento son varias escrituras seguidas.
func (h *furyHook) soon(app core.App) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(2*time.Second, func() { pushFury(app, false) })
}

func newSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "bfs_" + hex.EncodeToString(b)
}

func hookJSON(r *core.Record) map[string]any {
	if r == nil {
		return map[string]any{"url": "", "has_secret": false, "last_status": "", "last_at": "", "last_sent": 0}
	}
	return map[string]any{"url": r.GetString("url"), "has_secret": r.GetString("secret") != "",
		"last_status": r.GetString("last_status"), "last_at": r.GetString("last_at"), "last_sent": r.GetInt("last_sent")}
}

func registerFuryHook(app core.App) {
	// Lo que cambia la furia de alguien: se avisa a quien cambió.
	for _, col := range []string{"threads", "bubbles", "features", "fury_rules", "events", "users"} {
		changed := func(e *core.RecordEvent) error { furySent.soon(e.App); return e.Next() }
		app.OnRecordAfterCreateSuccess(col).BindFunc(changed)
		app.OnRecordAfterUpdateSuccess(col).BindFunc(changed)
		app.OnRecordAfterDeleteSuccess(col).BindFunc(changed)
	}
	// El latido, y lo que cambia solo con el tiempo (vencer, dormirse).
	app.Cron().MustAdd("bubbleFuryHeartbeat", "*/5 * * * *", func() { pushFury(app, true) })

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		lead := func(e *core.RequestEvent) error {
			if !isGlobalLead(e.Auth) {
				return e.ForbiddenError("el webhook de los semáforos es del lead global", nil)
			}
			return nil
		}
		se.Router.GET("/api/fury/hook", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			return e.JSON(http.StatusOK, hookJSON(hookRow(e.App)))
		}).Bind(apis.RequireAuth("users"))

		// Guardar la URL; la primera vez, o con `rotate`, nace un secreto nuevo
		// y es la ÚNICA vez que sale del servidor.
		se.Router.POST("/api/fury/hook", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			var body struct {
				URL    *string `json:"url"`
				Rotate bool    `json:"rotate"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("no se pudo leer", err)
			}
			row := hookRow(e.App)
			if row == nil {
				col, err := e.App.FindCollectionByNameOrId("fury_hook")
				if err != nil {
					return e.InternalServerError(err.Error(), err)
				}
				row = core.NewRecord(col)
			}
			if body.URL != nil {
				row.Set("url", *body.URL)
			}
			fresh := ""
			if body.Rotate || row.GetString("secret") == "" {
				fresh = newSecret()
				row.Set("secret", fresh)
			}
			if err := e.App.Save(row); err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			out := hookJSON(row)
			if fresh != "" {
				out["secret"] = fresh
			}
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth("users"))

		// Un latido ahora: para probar el receptor sin esperar 5 minutos.
		se.Router.POST("/api/fury/hook/test", func(e *core.RequestEvent) error {
			if err := lead(e); err != nil {
				return err
			}
			sent, failed, status := pushFury(e.App, true)
			out := hookJSON(hookRow(e.App))
			out["sent"], out["failed"], out["status"] = sent, failed, status
			return e.JSON(http.StatusOK, out)
		}).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}
