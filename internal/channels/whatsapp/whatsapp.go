// Package whatsapp es el canal de WhatsApp: los mensajes de UN grupo entran al
// inbox, y cada uno recibe una reacción 📥 al quedar capturado.
//
// Usa whatsmeow (go.mau.fi/whatsmeow), que habla el protocolo de WhatsApp Web
// multidispositivo — el mismo que Baileys, en Go. No es una API oficial: la
// cuenta vinculada puede ser bloqueada si se usa para mandar mensajes. Por eso
// este canal sólo ESCUCHA un grupo y contesta con una reacción, nunca escribe
// primero. Conviene vincular un número dedicado, no el de una persona.
//
// La sesión (las llaves de la cuenta vinculada) vive en su propio SQLite,
// `pb_data/channels/whatsapp.db`, con el mismo driver sin CGO que PocketBase.
package whatsapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "modernc.org/sqlite"

	"github.com/AngelMaldonado/bubble-work/internal/channels"
)

const kind = "whatsapp"

// La reacción con que se acusa recibo. Si reaccionar falla, se contesta con
// este texto en su lugar.
const (
	receivedEmoji = "📥"
	receivedText  = "📥 Recibido en el inbox"
)

func init() {
	channels.Register(kind, func(dataDir string, st channels.Store) channels.Channel {
		return &WhatsApp{dir: dataDir, store: st, state: "connecting"}
	})
}

// WhatsApp es el canal.
type WhatsApp struct {
	dir   string
	store channels.Store

	mu        sync.Mutex
	db        *sql.DB
	container *sqlstore.Container
	client    *whatsmeow.Client
	sink      channels.Sink
	state     string
	detail    string
	qr        string
	cancelQR  context.CancelFunc
	// Los álbumes que están llegando, por el id del mensaje álbum.
	albums map[string]*album
}

// album junta las fotos de un envío múltiple. WhatsApp no manda un mensaje con
// varias imágenes: manda un mensaje «álbum» (con cuántas vienen) y después cada
// foto como un mensaje propio que apunta a él. Sin juntarlas, cinco fotos de
// una misma tarea serían cinco notas en el inbox.
type album struct {
	in       channels.Inbound
	expected int
	got      int
	first    *events.Message
	timer    *time.Timer
}

// Cuánto se espera a la siguiente foto de un álbum antes de capturarlo con lo
// que llegó. Las fotos llegan casi juntas; esto cubre una conexión lenta.
const albumWait = 6 * time.Second

func (w *WhatsApp) Kind() string { return kind }

func (w *WhatsApp) set(state, detail string) {
	w.mu.Lock()
	w.state, w.detail = state, detail
	if state != "pairing" {
		w.qr = ""
	}
	w.mu.Unlock()
}

// Start abre la sesión guardada y conecta si ya hay una cuenta vinculada.
func (w *WhatsApp) Start(ctx context.Context, sink channels.Sink) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(w.dir, "whatsapp.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	container := sqlstore.NewWithDB(db, "sqlite", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("sesión de WhatsApp: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = db.Close()
		return err
	}

	w.mu.Lock()
	w.db, w.container, w.sink = db, container, sink
	w.mu.Unlock()
	w.newClient(device)

	if device.ID == nil {
		w.set("unlinked", "Vincula una cuenta de WhatsApp para empezar.")
		return nil
	}
	w.set("connecting", "")
	if err := w.client.Connect(); err != nil {
		w.set("error", err.Error())
	}
	return nil
}

func (w *WhatsApp) newClient(device *store.Device) {
	cli := whatsmeow.NewClient(device, waLog.Noop)
	cli.AddEventHandler(w.handle)
	w.mu.Lock()
	w.client = cli
	w.mu.Unlock()
}

// Stop desconecta y cierra la sesión.
func (w *WhatsApp) Stop() {
	w.mu.Lock()
	cli, db, cancel := w.client, w.db, w.cancelQR
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cli != nil {
		cli.Disconnect()
	}
	if db != nil {
		_ = db.Close()
	}
}

// Status dice cómo está, con la cuenta y el grupo que escucha.
func (w *WhatsApp) Status() channels.Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	cfg := w.store.Config()
	extra := map[string]any{
		"group_name": cfg["group_name"],
		"group_id":   cfg["group_id"],
	}
	if w.client != nil && w.client.Store != nil && w.client.Store.ID != nil {
		extra["account"] = "+" + w.client.Store.ID.User
		extra["push_name"] = w.client.Store.PushName
	}
	return channels.Status{State: w.state, Detail: w.detail, QR: w.qr, Extra: extra}
}

// Action: pair (QR) · pair-phone {phone} · groups · group {id|name} · unlink.
func (w *WhatsApp) Action(ctx context.Context, action string, body json.RawMessage) (any, error) {
	switch action {
	case "pair":
		return nil, w.pairQR()
	case "pair-phone":
		var in struct {
			Phone string `json:"phone"`
		}
		_ = json.Unmarshal(body, &in)
		code, err := w.pairPhone(ctx, in.Phone)
		if err != nil {
			return nil, err
		}
		return map[string]string{"code": code}, nil
	case "groups":
		return w.groups(ctx)
	case "group":
		var in struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(body, &in)
		return nil, w.chooseGroup(ctx, in.ID, in.Name)
	case "unlink":
		return nil, w.unlink(ctx)
	}
	return nil, fmt.Errorf("acción desconocida: %q", action)
}

// pairQR arranca la vinculación por QR. Los códigos rotan cada ~20 s; el
// estado lleva el vigente, y el manager lo sirve como imagen.
func (w *WhatsApp) pairQR() error {
	w.mu.Lock()
	cli := w.client
	w.mu.Unlock()
	if cli.Store.ID != nil {
		return errors.New("ya hay una cuenta vinculada: desvincúlala primero")
	}
	cli.Disconnect()
	qctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	ch, err := cli.GetQRChannel(qctx)
	if err != nil {
		cancel()
		return err
	}
	if err := cli.Connect(); err != nil {
		cancel()
		return err
	}
	w.mu.Lock()
	if w.cancelQR != nil {
		w.cancelQR()
	}
	w.cancelQR = cancel
	w.state, w.detail = "pairing", "Escanea el código desde WhatsApp → Dispositivos vinculados."
	w.mu.Unlock()

	go func() {
		defer cancel()
		for item := range ch {
			switch {
			case item.Event == whatsmeow.QRChannelEventCode:
				w.mu.Lock()
				w.qr, w.state = item.Code, "pairing"
				w.mu.Unlock()
			case item == whatsmeow.QRChannelSuccess:
				w.set("connecting", "Vinculado. Conectando…")
			case item == whatsmeow.QRChannelTimeout:
				w.set("unlinked", "El código caducó sin escanearse. Vuelve a intentarlo.")
				cli.Disconnect()
			case item.Event == whatsmeow.QRChannelEventError:
				w.set("unlinked", fmt.Sprintf("No se pudo vincular: %v", item.Error))
				cli.Disconnect()
			}
		}
	}()
	return nil
}

// pairPhone vincula con un código de 8 caracteres en vez de un QR: útil cuando
// el teléfono a vincular es el mismo en el que se mira la pantalla.
func (w *WhatsApp) pairPhone(ctx context.Context, phone string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if len(digits) < 8 {
		return "", errors.New("escribe el número completo, con lada de país (p. ej. 52 55 1234 5678)")
	}
	w.mu.Lock()
	cli := w.client
	w.mu.Unlock()
	if cli.Store.ID != nil {
		return "", errors.New("ya hay una cuenta vinculada: desvincúlala primero")
	}
	if !cli.IsConnected() {
		if err := cli.Connect(); err != nil {
			return "", err
		}
		// El socket tiene que estar listo antes de pedir el código.
		time.Sleep(2 * time.Second)
	}
	code, err := cli.PairPhone(ctx, digits, true, whatsmeow.PairClientChrome, "Chrome (Bubble Work)")
	if err != nil {
		return "", err
	}
	w.set("pairing", "Escribe el código en WhatsApp → Dispositivos vinculados → Vincular con número de teléfono.")
	return code, nil
}

type group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (w *WhatsApp) groups(ctx context.Context) ([]group, error) {
	w.mu.Lock()
	cli := w.client
	w.mu.Unlock()
	if cli.Store.ID == nil || !cli.IsLoggedIn() {
		return nil, errors.New("vincula y conecta la cuenta primero")
	}
	list, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]group, 0, len(list))
	for _, g := range list {
		out = append(out, group{ID: g.JID.String(), Name: g.Name})
	}
	return out, nil
}

// chooseGroup fija el grupo que se escucha, por id o por nombre. Por nombre
// tiene que ser uno solo: dos grupos con el mismo nombre no se adivinan.
func (w *WhatsApp) chooseGroup(ctx context.Context, id, name string) error {
	list, err := w.groups(ctx)
	if err != nil {
		return err
	}
	var hit []group
	for _, g := range list {
		if (id != "" && g.ID == id) || (id == "" && strings.EqualFold(strings.TrimSpace(g.Name), strings.TrimSpace(name))) {
			hit = append(hit, g)
		}
	}
	switch len(hit) {
	case 0:
		return fmt.Errorf("la cuenta vinculada no está en ningún grupo llamado %q: agrégala al grupo primero", name)
	case 1:
	default:
		return fmt.Errorf("hay %d grupos llamados %q: elígelo de la lista", len(hit), name)
	}
	cfg := w.store.Config()
	cfg["group_id"], cfg["group_name"] = hit[0].ID, hit[0].Name
	return w.store.SaveConfig(cfg)
}

// unlink cierra la sesión en WhatsApp y deja el canal listo para vincular otra.
func (w *WhatsApp) unlink(ctx context.Context) error {
	w.mu.Lock()
	cli, container := w.client, w.container
	w.mu.Unlock()
	if cli.Store.ID != nil {
		if err := cli.Logout(ctx); err != nil {
			// Si WhatsApp ya la había cerrado, igual se borra la local.
			_ = cli.Store.Delete(ctx)
		}
	}
	cli.Disconnect()
	w.newClient(container.NewDevice())
	w.set("unlinked", "Cuenta desvinculada.")
	return nil
}

// handle recibe los eventos de whatsmeow.
func (w *WhatsApp) handle(evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		w.set("connected", "")
	case *events.Disconnected:
		w.mu.Lock()
		if w.state == "connected" {
			w.state, w.detail = "connecting", "Reconectando…"
		}
		w.mu.Unlock()
	case *events.LoggedOut:
		w.set("unlinked", "WhatsApp cerró la sesión de este dispositivo. Vuelve a vincular.")
	case *events.StreamReplaced:
		w.set("error", "Otra instancia abrió esta misma sesión.")
	case *events.Message:
		w.message(v)
	}
}

// message captura un mensaje del grupo escuchado y le pone 📥.
func (w *WhatsApp) message(v *events.Message) {
	cfg := w.store.Config()
	groupID, _ := cfg["group_id"].(string)
	if groupID == "" || !v.Info.IsGroup || v.Info.Chat.String() != groupID {
		return
	}
	// Todo lo que llega del grupo, con lo que hace falta para saber por qué se
	// capturó o no: sin esto, «no se reflejó» no se puede diagnosticar.
	w.store.Logger().Debug("mensaje del grupo", "id", v.Info.ID, "from_me", v.Info.IsFromMe,
		"device", v.Info.Sender.Device, "edit", string(v.Info.Edit), "is_edit", v.IsEdit,
		"protocol", v.Message.GetProtocolMessage() != nil)
	w.mu.Lock()
	self := w.client.Store.ID
	w.mu.Unlock()
	// Lo propio sí entra: la cuenta vinculada puede ser la de quien escribe
	// tareas desde su teléfono, y esos mensajes llegan marcados como «míos».
	// Lo único que se ignora es lo que manda ESTE dispositivo —el acuse de
	// recibo—, que si no se capturaría a sí mismo.
	if v.Info.IsFromMe && self != nil && v.Info.Sender.Device == self.Device {
		return
	}
	msg := v.Message
	if msg == nil || msg.GetReactionMessage() != nil {
		return
	}
	// «Eliminar para todos»: llega como un mensaje de protocolo que nombra el
	// mensaje borrado. Lo capturado de él se deshace (ver `Retract`).
	if p := msg.GetProtocolMessage(); p != nil {
		w.store.Logger().Info("mensaje de protocolo", "type", p.GetType().String(), "ref", p.GetKey().GetID(),
			"edited_text", textOf(p.GetEditedMessage()))
		switch p.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			w.retract(p.GetKey().GetID())
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			// Editar: el mismo mensaje con otro texto. Sólo se reescribe el texto;
			// una edición no cambia las imágenes.
			w.revise(p.GetKey().GetID(), textOf(p.GetEditedMessage()))
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	w.mu.Lock()
	cli := w.client
	w.mu.Unlock()

	// Una edición, tal como la manda hoy WhatsApp: cifrada aparte con la clave
	// del mensaje original (`SecretEncryptedMessage`, tipo MESSAGE_EDIT), no como
	// un mensaje de protocolo en claro. Se descifra y se lleva al inbox.
	if enc := msg.GetSecretEncryptedMessage(); enc != nil {
		if enc.GetSecretEncType() != waE2E.SecretEncryptedMessage_MESSAGE_EDIT {
			return
		}
		target := enc.GetTargetMessageKey().GetID()
		dec, err := cli.DecryptSecretEncryptedMessage(ctx, v)
		if err != nil {
			w.store.Logger().Error("no se pudo descifrar una edición", "ref", target, "error", err)
			return
		}
		w.revise(target, editedText(dec))
		return
	}

	// El mensaje álbum no trae nada que capturar: dice cuántas fotos vienen.
	if a := msg.GetAlbumMessage(); a != nil {
		w.albumStart(v.Info.ID, int(a.GetExpectedImageCount()+a.GetExpectedVideoCount()))
		return
	}

	in := channels.Inbound{
		Ref:  v.Info.ID,
		From: sender(ctx, cli, v),
		At:   v.Info.Timestamp,
	}
	switch {
	case msg.GetConversation() != "":
		in.Text = msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		in.Text = msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		in.Text = msg.GetImageMessage().GetCaption()
		if data, err := cli.Download(ctx, msg.GetImageMessage()); err == nil {
			in.Images = append(in.Images, channels.Image{
				Name: "whatsapp" + ext(msg.GetImageMessage().GetMimetype()),
				Data: data,
				Ref:  v.Info.ID,
				Text: msg.GetImageMessage().GetCaption(),
			})
		}
	case msg.GetVideoMessage() != nil:
		in.Text = strings.TrimSpace(msg.GetVideoMessage().GetCaption() + "\n\n[video en WhatsApp: no se adjunta]")
	case msg.GetDocumentMessage() != nil:
		d := msg.GetDocumentMessage()
		in.Text = strings.TrimSpace(d.GetCaption() + "\n\n[documento en WhatsApp: " + d.GetFileName() + " — no se adjunta]")
	case msg.GetAudioMessage() != nil:
		in.Text = "[nota de voz en WhatsApp: no se adjunta]"
	default:
		return
	}

	// Una foto de un álbum se suma a él en vez de ser su propia nota.
	if assoc := msg.GetMessageContextInfo().GetMessageAssociation(); assoc != nil &&
		assoc.GetAssociationType() == waE2E.MessageAssociation_MEDIA_ALBUM &&
		assoc.GetParentMessageKey().GetID() != "" {
		w.albumAdd(assoc.GetParentMessageKey().GetID(), in, v)
		return
	}

	w.capture(ctx, in, v)
}

// capture da de alta la nota y acusa recibo sobre `v`.
func (w *WhatsApp) capture(ctx context.Context, in channels.Inbound, v *events.Message) {
	w.mu.Lock()
	cli, sink := w.client, w.sink
	w.mu.Unlock()
	err := sink.Capture(ctx, kind, in)
	if errors.Is(err, channels.ErrDuplicate) {
		w.store.Logger().Debug("ya capturado", "ref", in.Ref)
		return
	}
	if err != nil {
		w.store.Logger().Error("no se pudo capturar", "ref", in.Ref, "from", in.From, "error", err)
		// Sin 📥: quien escribió en el grupo no debe creer que entró. Y se dice
		// en la pantalla del canal, que es donde el lead lo va a buscar.
		w.mu.Lock()
		w.detail = fmt.Sprintf("Un mensaje de %s no se pudo capturar: %v", in.From, err)
		w.mu.Unlock()
		return
	}
	w.store.Logger().Info("capturado al inbox", "ref", in.Ref, "from", in.From, "images", len(in.Images))
	w.ack(ctx, cli, v)
}

// retract deshace lo capturado de un mensaje borrado en el grupo. Si era una
// foto de un álbum que todavía no se captura, sólo se saca del álbum.
func (w *WhatsApp) retract(id string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	for _, a := range w.albums {
		for i, img := range a.in.Images {
			if img.Ref == id {
				a.in.Images = append(a.in.Images[:i], a.in.Images[i+1:]...)
				w.mu.Unlock()
				return
			}
		}
	}
	sink := w.sink
	w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := sink.Retract(ctx, kind, id)
	w.store.Logger().Info("mensaje borrado en el grupo", "ref", id, "outcome", string(res), "error", err)
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case err != nil:
		w.detail = fmt.Sprintf("Se borró un mensaje en el grupo y no se pudo quitar del inbox: %v", err)
	case res == channels.OutcomeKept:
		w.detail = "Se borró en el grupo un mensaje cuya nota ya se trió: la nota se conserva."
	}
}

// revise lleva al inbox la edición de un mensaje. Si era una foto de un álbum
// que todavía no se captura, se cambia su pie dentro del álbum.
func (w *WhatsApp) revise(id, text string) {
	if id == "" {
		return
	}
	w.mu.Lock()
	for _, a := range w.albums {
		for i, img := range a.in.Images {
			if img.Ref == id {
				a.in.Images[i].Text = text
				var texts []string
				for _, x := range a.in.Images {
					if t := strings.TrimSpace(x.Text); t != "" {
						texts = append(texts, t)
					}
				}
				a.in.Text = strings.Join(texts, "\n\n")
				w.mu.Unlock()
				return
			}
		}
	}
	sink := w.sink
	w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := sink.Revise(ctx, kind, id, text)
	w.store.Logger().Info("mensaje editado en el grupo", "ref", id, "outcome", string(res), "error", err)
	w.mu.Lock()
	defer w.mu.Unlock()
	switch {
	case err != nil:
		w.detail = fmt.Sprintf("Se editó un mensaje en el grupo y no se pudo actualizar en el inbox: %v", err)
	case res == channels.OutcomeKept:
		w.detail = "Se editó en el grupo un mensaje cuya nota ya se trió: la nota se conserva como estaba."
	}
}

// editedText es el texto nuevo de una edición ya descifrada. Puede venir como
// el mensaje nuevo tal cual, o envuelto como en el formato viejo (un mensaje de
// protocolo con el mensaje editado dentro, a veces dentro de `EditedMessage`).
func editedText(m *waE2E.Message) string {
	if inner := m.GetEditedMessage().GetMessage(); inner != nil {
		m = inner
	}
	if p := m.GetProtocolMessage(); p != nil {
		return textOf(p.GetEditedMessage())
	}
	return textOf(m)
}

// textOf es el texto de un mensaje, sea cual sea su forma: texto simple, texto
// con formato o el pie de una imagen, un video o un documento.
func textOf(m *waE2E.Message) string {
	switch {
	case m == nil:
		return ""
	case m.GetConversation() != "":
		return m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetCaption()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetCaption()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetCaption()
	}
	return ""
}

func (w *WhatsApp) albumStart(id string, expected int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.albums == nil {
		w.albums = map[string]*album{}
	}
	a := w.albums[id]
	if a == nil {
		a = &album{}
		w.albums[id] = a
	}
	a.expected = expected
	w.armAlbum(id, a)
}

func (w *WhatsApp) albumAdd(id string, part channels.Inbound, v *events.Message) {
	w.mu.Lock()
	if w.albums == nil {
		w.albums = map[string]*album{}
	}
	a := w.albums[id]
	if a == nil {
		// La foto llegó antes que el mensaje álbum: se abre igual.
		a = &album{}
		w.albums[id] = a
	}
	if a.first == nil {
		a.first = v
		a.in = channels.Inbound{Ref: id, From: part.From, At: part.At}
	}
	if t := strings.TrimSpace(part.Text); t != "" {
		if a.in.Text != "" {
			a.in.Text += "\n\n"
		}
		a.in.Text += t
	}
	a.in.Images = append(a.in.Images, part.Images...)
	a.got++
	done := a.expected > 0 && a.got >= a.expected
	if !done {
		w.armAlbum(id, a)
	}
	w.mu.Unlock()
	if done {
		w.flushAlbum(id)
	}
}

// armAlbum (re)programa la captura del álbum por si no llegan todas las fotos.
// Se llama con `w.mu` tomado.
func (w *WhatsApp) armAlbum(id string, a *album) {
	if a.timer != nil {
		a.timer.Stop()
	}
	a.timer = time.AfterFunc(albumWait, func() { w.flushAlbum(id) })
}

func (w *WhatsApp) flushAlbum(id string) {
	w.mu.Lock()
	a := w.albums[id]
	delete(w.albums, id)
	w.mu.Unlock()
	if a == nil || a.first == nil {
		return
	}
	if a.timer != nil {
		a.timer.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	w.capture(ctx, a.in, a.first)
}

// ack acusa recibo con una reacción; si WhatsApp no la acepta, con un texto.
func (w *WhatsApp) ack(ctx context.Context, cli *whatsmeow.Client, v *events.Message) {
	reaction := cli.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, receivedEmoji)
	if _, err := cli.SendMessage(ctx, v.Info.Chat, reaction); err == nil {
		return
	}
	_, _ = cli.SendMessage(ctx, v.Info.Chat, &waE2E.Message{Conversation: proto.String(receivedText)})
}

// sender es quién lo mandó, como se lee: su nombre en WhatsApp y su número.
//
// En grupos WhatsApp identifica al remitente por un LID —un id interno que no
// es su teléfono— y a veces trae el número aparte (`SenderAlt`). Si no, se
// busca en el mapa LID → número que whatsmeow va guardando; sin ninguno de los
// dos se dice el nombre sin número antes que un número que no es de nadie.
func sender(ctx context.Context, cli *whatsmeow.Client, v *events.Message) string {
	num := ""
	switch {
	case v.Info.Sender.Server == types.DefaultUserServer:
		num = v.Info.Sender.User
	case v.Info.SenderAlt.Server == types.DefaultUserServer:
		num = v.Info.SenderAlt.User
	case v.Info.Sender.Server == types.HiddenUserServer:
		if pn, err := cli.Store.LIDs.GetPNForLID(ctx, v.Info.Sender.ToNonAD()); err == nil && !pn.IsEmpty() {
			num = pn.User
		} else if v.Info.IsFromMe && cli.Store.ID != nil {
			num = cli.Store.ID.User
		}
	}
	if num == "" {
		if v.Info.PushName != "" {
			return v.Info.PushName + " · WhatsApp"
		}
		return "alguien · WhatsApp"
	}
	if v.Info.PushName != "" {
		return fmt.Sprintf("%s (+%s) · WhatsApp", v.Info.PushName, num)
	}
	return fmt.Sprintf("+%s · WhatsApp", num)
}

func ext(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	return ".jpg"
}
