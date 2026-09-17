package bubble

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Tokens de agente.
//
// Un agente entra al MCP con un token suyo y no con la sesión de su persona: se
// revoca uno sin tocar los demás, caduca cuando se decidió (o nunca), y dice
// cuándo se usó por última vez. Actúa COMO su dueño —mismas reglas, misma
// firma—, así que no hay una identidad de agente que administrar.
//
// El token es `bw_` y 32 bytes aleatorios. Se guarda sólo su sha256: quien lea
// la base no tiene tokens, tiene huellas.

const tokenPrefix = "bw_"

// newToken devuelve el token en claro y su huella.
func newToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = tokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, hashToken(plain), nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// expiryIn: `days` días desde `from`, o sin caducidad con 0.
func expiryIn(from time.Time, days int) types.DateTime {
	if days <= 0 {
		return types.DateTime{}
	}
	dt, _ := types.ParseDateTime(from.Add(time.Duration(days) * 24 * time.Hour))
	return dt
}

// tokenAuthPriority: justo DESPUÉS de que PocketBase lea su propio token de
// sesión (los manejadores corren de menor a mayor prioridad), para que una
// sesión válida siga ganando y esto sólo mire lo que no era una.
const tokenAuthPriority = apis.DefaultLoadAuthTokenMiddlewarePriority + 1

func registerTokens(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id:       "bubbleAgentToken",
			Priority: tokenAuthPriority,
			Func:     agentTokenAuth,
		})

		se.Router.POST("/api/tokens", createToken).Bind(apis.RequireAuth("users"))
		se.Router.POST("/api/tokens/{id}/extend", extendToken).Bind(apis.RequireAuth("users"))
		se.Router.POST("/api/tokens/{id}/rename", renameToken).Bind(apis.RequireAuth("users"))
		se.Router.POST("/api/tokens/{id}/revoke", revokeToken).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}

// agentTokenAuth pone al dueño de un token de agente como quien pide.
//
// Sólo en /mcp: el token es la puerta de un agente, y el agente habla MCP. Que
// abriera también toda la API REST sería darle más de lo que se pidió al
// generarlo.
func agentTokenAuth(e *core.RequestEvent) error {
	if e.Auth != nil || !strings.HasPrefix(e.Request.URL.Path, "/mcp") {
		return e.Next()
	}
	raw := strings.TrimSpace(e.Request.Header.Get("Authorization"))
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "Bearer"))
	if !strings.HasPrefix(raw, tokenPrefix) {
		return e.Next()
	}
	owner, err := resolveAgentToken(e.App, raw)
	if err != nil {
		// Sin quien pida: `RequireAuth` contesta el 401, igual que con una
		// sesión caducada. No se dice por qué — un token revocado y uno
		// inventado se ven iguales desde fuera.
		return e.Next()
	}
	e.Auth = owner
	return e.Next()
}

// resolveAgentToken devuelve la persona de un token válido.
func resolveAgentToken(app core.App, plain string) (*core.Record, error) {
	tok, err := app.FindFirstRecordByData("agent_tokens", "hash", hashToken(plain))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if !tok.GetDateTime("revoked_at").IsZero() {
		return nil, errTokenUnusable
	}
	if exp := tok.GetDateTime("expires_at"); !exp.IsZero() && exp.Time().Before(now) {
		return nil, errTokenUnusable
	}
	owner, err := app.FindRecordById("users", tok.GetString("owner"))
	if err != nil {
		return nil, err
	}
	if !owner.GetDateTime("deleted_at").IsZero() {
		return nil, errTokenUnusable
	}
	// Una escritura por minuto como mucho, y directa: un agente hace decenas de
	// llamadas seguidas, y guardar el registro en cada una dispararía hooks y
	// tiempo real para decir «hace un momento» otra vez.
	if last := tok.GetDateTime("last_used_at"); last.IsZero() || now.Sub(last.Time()) > time.Minute {
		stamp, _ := types.ParseDateTime(now)
		_, _ = app.DB().NewQuery("UPDATE agent_tokens SET last_used_at = {:t} WHERE id = {:id}").
			Bind(dbx.Params{"t": stamp.String(), "id": tok.Id}).Execute()
	}
	return owner, nil
}

var errTokenUnusable = &tokenError{"token revocado, caducado o de una persona borrada"}

type tokenError struct{ msg string }

func (e *tokenError) Error() string { return e.msg }

// tokenJSON: lo que se enseña de un token. Nunca la huella.
func tokenJSON(r *core.Record) map[string]any {
	return map[string]any{
		"id":           r.Id,
		"owner":        r.GetString("owner"),
		"name":         r.GetString("name"),
		"prefix":       r.GetString("prefix"),
		"expires_at":   r.GetString("expires_at"),
		"last_used_at": r.GetString("last_used_at"),
		"revoked_at":   r.GetString("revoked_at"),
		"created":      r.GetString("created"),
	}
}

func createToken(e *core.RequestEvent) error {
	var body struct {
		Name string `json:"name"`
		Days int    `json:"days"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("cuerpo ilegible", err)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return e.BadRequestError("un token necesita un nombre: el del agente que lo va a usar", nil)
	}
	if body.Days < 0 {
		return e.BadRequestError("los días no pueden ser negativos; 0 es sin caducidad", nil)
	}
	col, err := e.App.FindCollectionByNameOrId("agent_tokens")
	if err != nil {
		return err
	}
	plain, hash, err := newToken()
	if err != nil {
		return err
	}
	r := core.NewRecord(col)
	r.Set("owner", e.Auth.Id)
	r.Set("name", name)
	r.Set("hash", hash)
	r.Set("prefix", plain[:len(tokenPrefix)+6])
	r.Set("expires_at", expiryIn(time.Now(), body.Days))
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	out := tokenJSON(r)
	// La única vez que el token sale del servidor.
	out["token"] = plain
	return e.JSON(http.StatusOK, out)
}

// ownToken carga un token que quien pide puede tocar: el suyo, o cualquiera si
// es el lead global. Los demás reciben 404: un token ajeno no existe para ellos.
func ownToken(e *core.RequestEvent, leadToo bool) (*core.Record, error) {
	r, err := e.App.FindRecordById("agent_tokens", e.Request.PathValue("id"))
	if err != nil {
		return nil, e.NotFoundError("", err)
	}
	if r.GetString("owner") != e.Auth.Id && !(leadToo && isGlobalLead(e.Auth)) {
		return nil, e.NotFoundError("", nil)
	}
	return r, nil
}

// extendToken alarga la caducidad `days` días desde hoy o desde la caducidad
// actual, la que sea más tarde; 0 la quita. Sólo el dueño: decidir cuánto vive
// la llave de un agente es de quien lo usa.
func extendToken(e *core.RequestEvent) error {
	r, err := ownToken(e, false)
	if err != nil {
		return err
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("cuerpo ilegible", err)
	}
	if body.Days < 0 {
		return e.BadRequestError("los días no pueden ser negativos; 0 es sin caducidad", nil)
	}
	if !r.GetDateTime("revoked_at").IsZero() {
		return e.BadRequestError("un token revocado no se extiende: genera otro", nil)
	}
	from := time.Now()
	if exp := r.GetDateTime("expires_at"); !exp.IsZero() && exp.Time().After(from) {
		from = exp.Time()
	}
	r.Set("expires_at", expiryIn(from, body.Days))
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, tokenJSON(r))
}

func renameToken(e *core.RequestEvent) error {
	r, err := ownToken(e, false)
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("cuerpo ilegible", err)
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return e.BadRequestError("un token necesita un nombre", nil)
	}
	r.Set("name", name)
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, tokenJSON(r))
}

// revokeToken: el dueño, o el lead global — que es quien corta la llave de
// alguien que ya no debería tenerla. No se deshace: se genera otro.
func revokeToken(e *core.RequestEvent) error {
	r, err := ownToken(e, true)
	if err != nil {
		return err
	}
	if r.GetDateTime("revoked_at").IsZero() {
		stamp, _ := types.ParseDateTime(time.Now())
		r.Set("revoked_at", stamp)
		if err := e.App.Save(r); err != nil {
			return e.BadRequestError(err.Error(), err)
		}
	}
	return e.JSON(http.StatusOK, tokenJSON(r))
}
