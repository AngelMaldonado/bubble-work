package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	qrcode "github.com/skip2/go-qrcode"
)

// ErrDuplicate: ese mensaje ya entró al inbox.
var ErrDuplicate = errors.New("ya capturado")

// Manager enciende, apaga y expone los canales compilados en el binario.
type Manager struct {
	app     core.App
	mu      sync.Mutex
	running map[string]Channel
}

// Mount monta los canales en la app: los enciende al arrancar el servidor si
// estaban encendidos, los apaga al terminar, y publica sus rutas.
func Mount(app core.App) *Manager {
	m := &Manager{app: app, running: map[string]Channel{}}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		for _, kind := range Kinds() {
			if row, err := m.row(kind); err == nil && row.GetBool("enabled") {
				if err := m.start(kind); err != nil {
					app.Logger().Error("canal no arrancó", "kind", kind, "error", err)
				}
			}
		}

		lead := apis.RequireAuth("users")
		se.Router.GET("/api/channels", m.list).Bind(lead)
		se.Router.POST("/api/channels/{kind}/enable", m.enable).Bind(lead)
		se.Router.POST("/api/channels/{kind}/disable", m.disable).Bind(lead)
		se.Router.GET("/api/channels/{kind}/qr.png", m.qr).Bind(lead)
		se.Router.POST("/api/channels/{kind}/{action}", m.action).Bind(lead)
		return se.Next()
	})
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		m.mu.Lock()
		for _, ch := range m.running {
			ch.Stop()
		}
		m.running = map[string]Channel{}
		m.mu.Unlock()
		return e.Next()
	})
	return m
}

// Los canales son del departamento: los conecta el lead global. Un canal
// decide qué entra al inbox de todos, y trae credenciales de una cuenta.
func onlyLead(e *core.RequestEvent) error {
	if e.Auth == nil || e.Auth.Collection().Name != "users" || e.Auth.GetString("role") != "lead" {
		return e.ForbiddenError("sólo el lead global conecta canales", nil)
	}
	return nil
}

// row trae (o crea, apagada) la fila de un tipo de canal.
func (m *Manager) row(kind string) (*core.Record, error) {
	if r, err := m.app.FindFirstRecordByData("channels", "kind", kind); err == nil {
		return r, nil
	}
	col, err := m.app.FindCollectionByNameOrId("channels")
	if err != nil {
		return nil, err
	}
	r := core.NewRecord(col)
	r.Set("kind", kind)
	r.Set("enabled", false)
	r.Set("config", map[string]any{})
	return r, m.app.Save(r)
}

type rowStore struct {
	m    *Manager
	kind string
}

func (s rowStore) Config() map[string]any {
	r, err := s.m.row(s.kind)
	out := map[string]any{}
	if err != nil {
		return out
	}
	_ = r.UnmarshalJSONField("config", &out)
	return out
}

func (s rowStore) Logger() *slog.Logger {
	return s.m.app.Logger().With("channel", s.kind)
}

func (s rowStore) SaveConfig(cfg map[string]any) error {
	r, err := s.m.row(s.kind)
	if err != nil {
		return err
	}
	r.Set("config", cfg)
	return s.m.app.Save(r)
}

func (m *Manager) start(kind string) error {
	f, ok := factory(kind)
	if !ok {
		return fmt.Errorf("este binario no trae el canal %q", kind)
	}
	m.mu.Lock()
	if _, on := m.running[kind]; on {
		m.mu.Unlock()
		return nil
	}
	dir := filepath.Join(m.app.DataDir(), "channels")
	ch := f(dir, rowStore{m, kind})
	m.running[kind] = ch
	m.mu.Unlock()
	return ch.Start(context.Background(), &sink{m.app, kind})
}

func (m *Manager) stop(kind string) {
	m.mu.Lock()
	ch := m.running[kind]
	delete(m.running, kind)
	m.mu.Unlock()
	if ch != nil {
		ch.Stop()
	}
}

func (m *Manager) status(kind string) Status {
	m.mu.Lock()
	ch := m.running[kind]
	m.mu.Unlock()
	if ch != nil {
		st := ch.Status()
		st.Kind, st.Enabled = kind, true
		return st
	}
	return Status{Kind: kind, State: "disabled"}
}

func (m *Manager) list(e *core.RequestEvent) error {
	if err := onlyLead(e); err != nil {
		return err
	}
	out := []Status{}
	for _, kind := range Kinds() {
		out = append(out, m.status(kind))
	}
	return e.JSON(http.StatusOK, out)
}

func (m *Manager) enable(e *core.RequestEvent) error {
	if err := onlyLead(e); err != nil {
		return err
	}
	kind := e.Request.PathValue("kind")
	if _, ok := factory(kind); !ok {
		return e.NotFoundError("", nil)
	}
	r, err := m.row(kind)
	if err != nil {
		return err
	}
	r.Set("enabled", true)
	// Lo que entra se captura a nombre de quien conecta el canal.
	r.Set("owner", e.Auth.Id)
	if err := m.app.Save(r); err != nil {
		return err
	}
	if err := m.start(kind); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, m.status(kind))
}

func (m *Manager) disable(e *core.RequestEvent) error {
	if err := onlyLead(e); err != nil {
		return err
	}
	kind := e.Request.PathValue("kind")
	r, err := m.row(kind)
	if err != nil {
		return e.NotFoundError("", err)
	}
	r.Set("enabled", false)
	if err := m.app.Save(r); err != nil {
		return err
	}
	m.stop(kind)
	return e.JSON(http.StatusOK, m.status(kind))
}

// qr sirve el código de vinculación como PNG. Se genera en el servidor para
// que el cliente no cargue una librería de QR sólo para esta pantalla.
func (m *Manager) qr(e *core.RequestEvent) error {
	if err := onlyLead(e); err != nil {
		return err
	}
	st := m.status(e.Request.PathValue("kind"))
	if st.QR == "" {
		return e.NotFoundError("no hay un código que escanear ahora", nil)
	}
	png, err := qrcode.Encode(st.QR, qrcode.Medium, 320)
	if err != nil {
		return err
	}
	h := e.Response.Header()
	h.Set("Content-Type", "image/png")
	h.Set("Cache-Control", "no-store")
	_, err = e.Response.Write(png)
	return err
}

func (m *Manager) action(e *core.RequestEvent) error {
	if err := onlyLead(e); err != nil {
		return err
	}
	kind := e.Request.PathValue("kind")
	m.mu.Lock()
	ch := m.running[kind]
	m.mu.Unlock()
	if ch == nil {
		return e.BadRequestError("el canal está apagado: enciéndelo primero", nil)
	}
	var buf bytes.Buffer
	if e.Request.Body != nil {
		_, _ = buf.ReadFrom(http.MaxBytesReader(e.Response, e.Request.Body, 64<<10))
	}
	out, err := ch.Action(e.Request.Context(), e.Request.PathValue("action"), json.RawMessage(buf.Bytes()))
	if err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	if out == nil {
		out = m.status(kind)
	}
	return e.JSON(http.StatusOK, out)
}

// sink convierte un mensaje en una nota del inbox.
type sink struct {
	app  core.App
	kind string
}

// noteMax es el tope de `inbox_items.note`: la primera línea va ahí, entera en
// el cuerpo.
const noteMax = 200

func (s *sink) Capture(_ context.Context, kind string, in Inbound) error {
	if in.Ref != "" {
		if _, err := s.app.FindFirstRecordByFilter("inbox_items",
			"source = {:k} && source_ref = {:r}", map[string]any{"k": kind, "r": in.Ref}); err == nil {
			return ErrDuplicate
		}
	}
	ch, err := s.app.FindFirstRecordByData("channels", "kind", kind)
	if err != nil {
		return err
	}
	owner := ch.GetString("owner")
	if owner == "" {
		return errors.New("el canal no tiene a nombre de quién capturar: vuelve a encenderlo")
	}
	col, err := s.app.FindCollectionByNameOrId("inbox_items")
	if err != nil {
		return err
	}

	text := strings.TrimSpace(in.Text)
	note := firstLine(text)
	if note == "" {
		note = fmt.Sprintf("Imagen de %s", in.From)
	}
	r := core.NewRecord(col)
	r.Set("note", note)
	r.Set("body", text)
	r.Set("captured_by", owner)
	r.Set("source", kind)
	r.Set("source_ref", in.Ref)
	r.Set("source_from", truncate(in.From, 200))
	var files []*filesystem.File
	for _, img := range in.Images {
		f, err := filesystem.NewFileFromBytes(img.Data, img.Name)
		if err == nil {
			files = append(files, f)
		}
	}
	if len(files) > 0 {
		r.Set("files", files)
	}
	if err := s.app.Save(r); err != nil {
		return err
	}
	// Qué mensaje trajo cada archivo: PocketBase guarda los archivos en el orden
	// en que se dieron, así que el nombre final i-ésimo es la imagen i-ésima.
	names := r.GetStringSlice("files")
	var parts []map[string]string
	for i, name := range names {
		if i < len(in.Images) && in.Images[i].Ref != "" {
			parts = append(parts, map[string]string{"ref": in.Images[i].Ref, "file": name, "text": in.Images[i].Text})
		}
	}
	if len(parts) > 0 {
		r.Set("source_parts", parts)
	}
	// Las imágenes se CITAN en el cuerpo, igual que al pegarlas en una nota: la
	// nota enseña su markdown, y un archivo adjunto que nadie cita no se ve.
	// Sólo se sabe su nombre final después de guardar —PocketBase le añade un
	// sufijo—, así que va en una segunda escritura.
	if len(names) > 0 {
		var b strings.Builder
		b.WriteString(text)
		for _, name := range names {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "![imagen](/api/files/inbox_items/%s/%s)", r.Id, name)
		}
		r.Set("body", b.String())
		return s.app.Save(r)
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncate(strings.TrimSpace(s), noteMax)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// Retract deshace lo capturado de un mensaje borrado en el canal.
//
// Si el mensaje es el de la nota entera, se borra la nota. Si es una pieza —una
// foto de un álbum—, se quita esa imagen, su cita en el cuerpo y su pieza; si la
// nota se queda sin imágenes ni texto, se borra. Una nota ya triada (convertida
// en hilo o burbuja) no se toca: sobre ella ya hay trabajo, y borrar un mensaje
// en un chat no es decidir que ese trabajo sobra.
func (s *sink) Retract(_ context.Context, kind, ref string) (Outcome, error) {
	if ref == "" {
		return OutcomeNone, nil
	}
	r, err := s.app.FindFirstRecordByFilter("inbox_items",
		"source = {:k} && (source_ref = {:r} || source_parts ~ {:r})",
		map[string]any{"k": kind, "r": ref})
	if err != nil {
		return OutcomeNone, nil
	}
	if r.GetString("thread") != "" || r.GetString("bubble") != "" {
		return OutcomeKept, nil
	}
	if r.GetString("source_ref") == ref {
		return OutcomeDeleted, s.app.Delete(r)
	}

	var parts []map[string]string
	_ = r.UnmarshalJSONField("source_parts", &parts)
	file := ""
	kept := parts[:0]
	for _, p := range parts {
		if p["ref"] == ref {
			file = p["file"]
			continue
		}
		kept = append(kept, p)
	}
	if file == "" {
		return OutcomeNone, nil
	}
	files := []string{}
	for _, f := range r.GetStringSlice("files") {
		if f != file {
			files = append(files, f)
		}
	}
	cite := fmt.Sprintf("![imagen](/api/files/inbox_items/%s/%s)", r.Id, file)
	body := strings.TrimSpace(strings.ReplaceAll(r.GetString("body"), cite, ""))
	for strings.Contains(body, "\n\n\n") {
		body = strings.ReplaceAll(body, "\n\n\n", "\n\n")
	}
	if len(files) == 0 && body == "" {
		return OutcomeDeleted, s.app.Delete(r)
	}
	// `files-` quita ese archivo del registro y del almacenamiento.
	r.Set("files-", []string{file})
	r.Set("body", body)
	r.Set("source_parts", kept)
	return OutcomeTrimmed, s.app.Save(r)
}

// Revise reescribe el texto de un mensaje editado en el canal.
//
// Si el mensaje es el de la nota, su texto pasa a ser el nuevo. Si es una pieza
// (el pie de una foto de un álbum), se reescribe ese pie y el texto de la nota
// se vuelve a componer con los de todas las piezas. Las imágenes se quedan
// citadas debajo, como al capturar.
func (s *sink) Revise(_ context.Context, kind, ref, text string) (Outcome, error) {
	if ref == "" {
		return OutcomeNone, nil
	}
	r, err := s.app.FindFirstRecordByFilter("inbox_items",
		"source = {:k} && (source_ref = {:r} || source_parts ~ {:r})",
		map[string]any{"k": kind, "r": ref})
	if err != nil {
		return OutcomeNone, nil
	}
	if r.GetString("thread") != "" || r.GetString("bubble") != "" {
		return OutcomeKept, nil
	}
	text = strings.TrimSpace(text)

	var parts []map[string]string
	_ = r.UnmarshalJSONField("source_parts", &parts)
	matched := false
	for _, p := range parts {
		if p["ref"] == ref {
			p["text"] = text
			matched = true
		}
	}
	if matched {
		var texts []string
		for _, p := range parts {
			if t := strings.TrimSpace(p["text"]); t != "" {
				texts = append(texts, t)
			}
		}
		text = strings.Join(texts, "\n\n")
		r.Set("source_parts", parts)
	} else if r.GetString("source_ref") != ref {
		return OutcomeNone, nil
	}

	var b strings.Builder
	b.WriteString(text)
	for _, name := range r.GetStringSlice("files") {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "![imagen](/api/files/inbox_items/%s/%s)", r.Id, name)
	}
	note := firstLine(text)
	if note == "" {
		note = r.GetString("note")
	}
	r.Set("note", note)
	r.Set("body", b.String())
	return OutcomeUpdated, s.app.Save(r)
}
