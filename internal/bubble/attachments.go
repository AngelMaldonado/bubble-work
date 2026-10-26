package bubble

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/security"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decodifica webp para reducirla

	"github.com/AngelMaldonado/bubble-work/internal/tree"
)

// Adjuntos para el agente.
//
// Un adjunto vive en uno de cuatro lugares: `assets/` de un proyecto (en git),
// los archivos de una idea del inbox, los de Mi espacio y la imagen de algo del
// inventario. El agente entra sólo por `/mcp`, así que hasta aquí no podía ni
// verlos ni subir uno. Ahora:
//
//   - VER: `attachment_read` manda la imagen como contenido `image` (reducida un
//     poco), un texto como texto, y SIEMPRE un enlace de descarga de un solo uso:
//     no todos los harness pasan una imagen de una tool al modelo, pero todos
//     tienen terminal y saben leer un archivo del disco.
//   - SUBIR: `attachment_upload` da un enlace firmado de un solo uso al que el
//     agente sube con `curl -F`. Los bytes no pasan por el modelo (en base64,
//     una imagen de 1 MB serían cientos de miles de tokens) y el token del agente
//     no tiene que estar en la terminal.
//
// Las reglas son las de siempre, repetidas aquí como en cada puerta del agente:
// un proyecto que ves, una idea tuya o de un equipo que lideras, Mi espacio sólo
// de su dueño, el inventario si te lo asignaron.

// AttachRef dice dónde está un adjunto. Uno de cuatro, con sus campos.
type AttachRef struct {
	Project string `json:"project,omitempty"` // + Path
	Path    string `json:"path,omitempty"`    // assets/…
	Idea    string `json:"idea,omitempty"`    // + File
	File    string `json:"file,omitempty"`    // el nombre que da inbox_read
	Space   string `json:"space,omitempty"`   // el id de un archivo de Mi espacio
	Inv     string `json:"inventory,omitempty"`
}

// Attachment es un adjunto leído.
type Attachment struct {
	Name  string
	MIME  string
	Bytes []byte
	Where string // dónde está, en palabras
}

// AttachInfo es una fila de `attachment_list`.
type AttachInfo struct {
	Ref   AttachRef `json:"ref"`
	Name  string    `json:"name"`
	Image bool      `json:"image"`
	Bytes int64     `json:"bytes,omitempty"`
	Note  string    `json:"note,omitempty"` // la nota de Mi espacio a la que va pegado
}

func (r AttachRef) kind() string {
	switch {
	case r.Project != "" && r.Path != "":
		return "project"
	case r.Idea != "" && r.File != "":
		return "idea"
	case r.Space != "":
		return "space"
	case r.Inv != "":
		return "inventory"
	}
	return ""
}

var errWhich = fmt.Errorf("di cuál adjunto: `project`+`path` (assets/…), `idea`+`file`, `space` o `inventory`")

// ReadAttachment lee un adjunto, con las reglas del lugar donde está.
func ReadAttachment(app core.App, auth *core.Record, t *tree.Tree, ref AttachRef) (Attachment, error) {
	if !isPersonAuth(auth) {
		return Attachment{}, ErrNotAPerson
	}
	switch ref.kind() {
	case "project":
		ws, repo, err := WorkspaceFor(app, auth, ref.Project)
		if err != nil {
			return Attachment{}, err
		}
		p := path.Clean(strings.TrimPrefix(ref.Path, "/"))
		if !strings.HasPrefix(p, tree.DirAssets+"/") || !tree.IsAttachment(p) {
			return Attachment{}, fmt.Errorf("un adjunto de proyecto vive en assets/ (imágenes, pdf, txt, csv, json)")
		}
		body, err := t.ReadBytes(repo, p)
		if err != nil {
			return Attachment{}, fmt.Errorf("no hay %s en %s", p, ws.GetString("slug"))
		}
		return Attachment{Name: path.Base(p), MIME: mimeOf(p), Bytes: body, Where: ws.GetString("slug") + "/" + p}, nil
	case "idea":
		r, err := ideaFor(app, auth, ref.Idea)
		if err != nil {
			return Attachment{}, err
		}
		if !contains(r.GetStringSlice("files"), ref.File) {
			return Attachment{}, fmt.Errorf("la idea no tiene un archivo %q: inbox_read dice cuáles tiene", ref.File)
		}
		return recordFile(app, r, ref.File, "idea «"+r.GetString("note")+"»")
	case "space":
		r, err := app.FindRecordById("personal_assets", ref.Space)
		if err != nil || r.GetString("owner") != auth.Id {
			return Attachment{}, ErrDenied
		}
		a, err := recordFile(app, r, r.GetString("file"), "Mi espacio")
		if err == nil && r.GetString("name") != "" {
			a.Name = r.GetString("name")
		}
		return a, err
	case "inventory":
		if !SeesInventory(app, auth) {
			return Attachment{}, ErrDenied
		}
		for _, col := range []string{"inventory_items", "inventory_groups"} {
			if r, err := app.FindRecordById(col, ref.Inv); err == nil {
				if r.GetString("image") == "" {
					return Attachment{}, fmt.Errorf("«%s» no tiene imagen", r.GetString("name"))
				}
				return recordFile(app, r, r.GetString("image"), "inventario «"+r.GetString("name")+"»")
			}
		}
		return Attachment{}, ErrDenied
	}
	return Attachment{}, errWhich
}

// ListAttachments: los adjuntos de un lugar. `where` es project, idea, space o
// inventory; project e idea piden su id.
func ListAttachments(app core.App, auth *core.Record, t *tree.Tree, where, id string) ([]AttachInfo, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	out := []AttachInfo{}
	switch where {
	case "project":
		ws, repo, err := WorkspaceFor(app, auth, id)
		if err != nil {
			return nil, err
		}
		entries, err := t.Tree(repo)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Path, tree.DirAssets+"/") && tree.IsAttachment(e.Path) {
				out = append(out, AttachInfo{Ref: AttachRef{Project: ws.GetString("slug"), Path: e.Path},
					Name: path.Base(e.Path), Image: tree.IsImage(e.Path)})
			}
		}
	case "idea":
		r, err := ideaFor(app, auth, id)
		if err != nil {
			return nil, err
		}
		for _, f := range r.GetStringSlice("files") {
			out = append(out, AttachInfo{Ref: AttachRef{Idea: r.Id, File: f}, Name: f, Image: true})
		}
	case "space":
		rows, err := app.FindRecordsByFilter("personal_assets", "owner = {:me}", "-created", 0, 0, map[string]any{"me": auth.Id})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			name := r.GetString("name")
			out = append(out, AttachInfo{Ref: AttachRef{Space: r.Id}, Name: name, Image: tree.IsImage(name), Note: r.GetString("note")})
		}
	case "inventory":
		if !SeesInventory(app, auth) {
			return nil, ErrDenied
		}
		for _, col := range []string{"inventory_groups", "inventory_items"} {
			rows, err := app.FindAllRecords(col)
			if err != nil {
				continue
			}
			for _, r := range rows {
				if r.GetString("image") != "" {
					out = append(out, AttachInfo{Ref: AttachRef{Inv: r.Id}, Name: r.GetString("name"), Image: true})
				}
			}
		}
	default:
		return nil, fmt.Errorf("`where` es project, idea, space o inventory")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func recordFile(app core.App, r *core.Record, name, where string) (Attachment, error) {
	fsys, err := app.NewFilesystem()
	if err != nil {
		return Attachment{}, err
	}
	defer fsys.Close()
	rd, err := fsys.GetReader(r.BaseFilesPath() + "/" + name)
	if err != nil {
		return Attachment{}, fmt.Errorf("el archivo %s ya no está", name)
	}
	defer rd.Close()
	body, err := io.ReadAll(io.LimitReader(rd, 64<<20))
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Name: name, MIME: mimeOf(name), Bytes: body, Where: where}, nil
}

func mimeOf(name string) string {
	if ct := mime.TypeByExtension(strings.ToLower(path.Ext(name))); ct != "" {
		return strings.Split(ct, ";")[0]
	}
	return "application/octet-stream"
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ---- reducción ligera -------------------------------------------------------

// maxEdge: el lado largo con el que una imagen llega al modelo. Ligera: los
// modelos de visión trabajan alrededor de 1500–2000 px y reducen solos lo que
// pasa de ahí; mandar más es pagar contexto por píxeles que no se miran.
const maxEdge = 2000

// maxInline: lo que se manda tal cual, sin reencodear, si ya cabe en maxEdge.
const maxInline = 2 << 20

// ForModel prepara una imagen para mandarla como contenido `image`: la original
// si ya es chica, si no reducida a maxEdge y en PNG (con transparencia o pocas
// tintas) o JPEG. `ok` es false si no se puede decodificar (AVIF, rota): queda
// la descarga.
func ForModel(a Attachment) (body []byte, mimeType string, ok bool) {
	switch a.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return nil, "", false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(a.Bytes))
	if err != nil {
		return nil, "", false
	}
	small := cfg.Width <= maxEdge && cfg.Height <= maxEdge
	if small && len(a.Bytes) <= maxInline && a.MIME != "image/gif" {
		return a.Bytes, a.MIME, true
	}
	src, _, err := image.Decode(bytes.NewReader(a.Bytes)) // un GIF animado: su primer cuadro
	if err != nil {
		return nil, "", false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if !small {
		if w >= h {
			h, w = h*maxEdge/w, maxEdge
		} else {
			w, h = w*maxEdge/h, maxEdge
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
		src = dst
	}
	var buf bytes.Buffer
	if a.MIME != "image/jpeg" {
		if err := png.Encode(&buf, src); err == nil && buf.Len() <= maxInline {
			return buf.Bytes(), "image/png", true
		}
		buf.Reset()
	}
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", false
	}
	return buf.Bytes(), "image/jpeg", true
}

var _ = gif.Decode // registra el decodificador de GIF

// IsText dice si un adjunto se puede mandar como texto.
func IsText(a Attachment) bool {
	switch a.MIME {
	case "text/plain", "text/csv", "application/json", "image/svg+xml", "text/markdown":
		return true
	}
	return strings.HasPrefix(a.MIME, "text/")
}

// ---- enlaces de un solo uso -------------------------------------------------

// linkTTL: cuánto vale un enlace. Lo que tarda un agente en correr el curl que
// se le acaba de dar, con holgura.
const linkTTL = 10 * time.Minute

type oneLink struct {
	user    string    // quién lo pidió: se re-comprueba al usarlo
	expires time.Time // vence solo
	get     *AttachRef
	put     *UploadTarget
}

// UploadTarget dice a dónde va lo que se sube.
type UploadTarget struct {
	Project string `json:"project,omitempty"` // a assets/ del proyecto
	Idea    string `json:"idea,omitempty"`    // a los archivos de una idea
	Space   bool   `json:"space,omitempty"`   // a Mi espacio
	Note    string `json:"note,omitempty"`    // …pegado a esta nota
	Name    string `json:"name,omitempty"`    // el nombre con el que se guarda
}

var links = struct {
	sync.Mutex
	m map[string]oneLink
}{m: map[string]oneLink{}}

func newLink(l oneLink) string {
	key := security.RandomString(40)
	links.Lock()
	defer links.Unlock()
	now := time.Now()
	for k, v := range links.m { // los vencidos se van al pedir otro
		if now.After(v.expires) {
			delete(links.m, k)
		}
	}
	links.m[key] = l
	return key
}

// takeLink consume un enlace: vale una vez.
func takeLink(key string) (oneLink, bool) {
	links.Lock()
	defer links.Unlock()
	for k, v := range links.m {
		if subtle.ConstantTimeCompare([]byte(k), []byte(key)) == 1 {
			delete(links.m, k)
			if time.Now().After(v.expires) {
				return oneLink{}, false
			}
			return v, true
		}
	}
	return oneLink{}, false
}

// DownloadLink comprueba que la persona ve el adjunto y da un enlace para bajarlo.
func DownloadLink(app core.App, auth *core.Record, t *tree.Tree, base string, ref AttachRef) (string, Attachment, error) {
	a, err := ReadAttachment(app, auth, t, ref)
	if err != nil {
		return "", Attachment{}, err
	}
	key := newLink(oneLink{user: auth.Id, expires: time.Now().Add(linkTTL), get: &ref})
	return strings.TrimRight(base, "/") + "/api/attachments/" + key, a, nil
}

// UploadLink comprueba que la persona puede escribir ahí y da un enlace para subir.
func UploadLink(app core.App, auth *core.Record, base string, to UploadTarget) (string, error) {
	if !isPersonAuth(auth) {
		return "", ErrNotAPerson
	}
	n := 0
	if to.Project != "" {
		n++
		w, _, err := WorkspaceFor(app, auth, to.Project)
		if err != nil {
			return "", err
		}
		to.Project = w.Id
	}
	if to.Idea != "" {
		n++
		if _, err := ideaFor(app, auth, to.Idea); err != nil {
			return "", err
		}
	}
	if to.Space {
		n++
		if to.Note != "" {
			if _, err := noteFor(app, auth, to.Note); err != nil {
				return "", err
			}
		}
	}
	if n != 1 {
		return "", fmt.Errorf("di a dónde se sube: `project`, `idea` o `space`")
	}
	key := newLink(oneLink{user: auth.Id, expires: time.Now().Add(linkTTL), put: &to})
	return strings.TrimRight(base, "/") + "/api/attachments/" + key, nil
}

// whoFor re-lee a quien pidió el enlace: si la borraron entretanto, ya no vale.
func whoFor(app core.App, l oneLink) (*core.Record, error) {
	u, err := app.FindRecordById("users", l.user)
	if err != nil || !u.GetDateTime("deleted_at").IsZero() {
		return nil, ErrDenied
	}
	return u, nil
}

func registerAttachments(app core.App, t *tree.Tree) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// Sin sesión: el enlace ES la credencial, de un solo uso y de 10 minutos.
		se.Router.GET("/api/attachments/{key}", func(e *core.RequestEvent) error {
			l, ok := takeLink(e.Request.PathValue("key"))
			if !ok || l.get == nil {
				return e.NotFoundError("este enlace ya se usó o venció: pide otro con attachment_read", nil)
			}
			u, err := whoFor(e.App, l)
			if err != nil {
				return e.NotFoundError("", nil)
			}
			a, err := ReadAttachment(e.App, u, t, *l.get)
			if err != nil {
				return e.NotFoundError("", err)
			}
			h := e.Response.Header()
			h.Set("Content-Type", a.MIME)
			h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Name))
			h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Cache-Control", "no-store")
			_, werr := e.Response.Write(a.Bytes)
			return werr
		})

		se.Router.POST("/api/attachments/{key}", func(e *core.RequestEvent) error {
			l, ok := takeLink(e.Request.PathValue("key"))
			if !ok || l.put == nil {
				return e.NotFoundError("este enlace ya se usó o venció: pide otro con attachment_upload", nil)
			}
			u, err := whoFor(e.App, l)
			if err != nil {
				return e.NotFoundError("", nil)
			}
			limit := int64(20 << 20)
			if l.put.Project != "" {
				limit = assetLimit()
			}
			e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, limit+1024)
			if err := e.Request.ParseMultipartForm(limit + 1024); err != nil {
				return e.BadRequestError(fmt.Sprintf("no se pudo leer el archivo (máximo %d bytes)", limit), err)
			}
			file, header, err := e.Request.FormFile("file")
			if err != nil {
				return e.BadRequestError("manda el archivo como `file`: curl -F file=@ruta", err)
			}
			defer file.Close()
			body, err := io.ReadAll(io.LimitReader(file, limit+1))
			if err != nil || int64(len(body)) > limit {
				return e.BadRequestError(fmt.Sprintf("el archivo pasa de %d bytes", limit), err)
			}
			name := strings.TrimSpace(l.put.Name)
			if name == "" {
				name = header.Filename
			}
			out, err := storeUpload(e.App, u, t, *l.put, slugifyFile(name), body)
			if err != nil {
				return e.BadRequestError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, out)
		})
		return se.Next()
	})
}

// storeUpload guarda lo subido donde dice el enlace, como lo haría la app.
func storeUpload(app core.App, u *core.Record, t *tree.Tree, to UploadTarget, name string, body []byte) (map[string]any, error) {
	switch {
	case to.Project != "":
		ws, repo, err := WorkspaceFor(app, u, to.Project)
		if err != nil {
			return nil, err
		}
		p := path.Join(tree.DirAssets, name)
		if !tree.IsAttachment(p) {
			return nil, fmt.Errorf("assets/ guarda imágenes (png, jpg, gif, webp, svg, avif) y documentos (pdf, txt, csv, json)")
		}
		p, err = placeAsset(t, repo, p, body, actorLabel(u))
		if err != nil {
			return nil, err
		}
		cite := "[" + path.Base(p) + "](" + p + ")"
		if tree.IsImage(p) {
			cite = "!" + cite
		}
		// Una imagen no es producción: no calienta nada. Lo que la usa, sí.
		return map[string]any{"project": ws.GetString("slug"), "path": p, "bytes": len(body),
			"markdown": cite, "next": "cítalo en el documento del hilo con document_edit: " + cite}, nil
	case to.Idea != "":
		r, err := ideaFor(app, u, to.Idea)
		if err != nil {
			return nil, err
		}
		before := r.GetStringSlice("files")
		f, err := filesystem.NewFileFromBytes(body, name)
		if err != nil {
			return nil, err
		}
		r.Set("files+", f)
		if err := app.Save(r); err != nil {
			return nil, fmt.Errorf("la idea no aceptó el archivo (png, jpg, webp, gif o avif, hasta 10 MB): %w", err)
		}
		added := ""
		for _, n := range r.GetStringSlice("files") {
			if !contains(before, n) {
				added = n
			}
		}
		return map[string]any{"idea": r.Id, "file": added, "bytes": len(body)}, nil
	default:
		col, err := app.FindCollectionByNameOrId("personal_assets")
		if err != nil {
			return nil, err
		}
		f, err := filesystem.NewFileFromBytes(body, name)
		if err != nil {
			return nil, err
		}
		r := core.NewRecord(col)
		r.Set("owner", u.Id)
		r.Set("name", name)
		r.Set("file", f)
		if to.Note != "" {
			r.Set("note", to.Note)
		}
		if err := app.Save(r); err != nil {
			return nil, err
		}
		return map[string]any{"space": r.Id, "name": name, "bytes": len(body), "note": to.Note}, nil
	}
}
