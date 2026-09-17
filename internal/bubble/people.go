package bubble

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Gestión de personas: el lead global da de alta, cambia el rol, borra y
// restaura.
//
// Hasta aquí una persona sólo se creaba desde la terminal (`just person`), y no
// se podía borrar: un comentario exige autor. Borrar es MARCAR (`deleted_at`),
// ver la migración `soft_delete_users` para lo que eso implica.
//
// Rutas propias y no la API de registros para lo que tiene candados que una
// regla no sabe expresar: una regla contesta «¿puedes tocar esta fila?», nunca
// «¿cuántos leads globales quedarían?».

func registerPeople(app core.App) {
	app.OnRecordUpdateRequest("users").BindFunc(keepAGlobalLead)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/people", createPerson).Bind(apis.RequireAuth("users"))
		se.Router.POST("/api/people/{id}/delete", softDeletePerson).Bind(apis.RequireAuth("users"))
		se.Router.POST("/api/people/{id}/restore", restorePerson).Bind(apis.RequireAuth("users"))
		return se.Next()
	})
}

// liveLeads cuenta los leads globales que no están borrados.
func liveLeads(app core.App) (int, error) {
	var n int
	err := app.DB().Select("count(*)").From("users").
		Where(dbx.HashExp{"role": "lead"}).
		AndWhere(dbx.NewExp("(deleted_at = '' OR deleted_at IS NULL)")).
		Row(&n)
	return n, err
}

// keepAGlobalLead: quitarse el rol a uno mismo, o quitárselo al último lead
// global, deja el departamento sin nadie que dé forma al plan ni gestione a las
// personas — y sólo un superuser desde el dashboard podría arreglarlo.
func keepAGlobalLead(e *core.RecordRequestEvent) error {
	info, err := e.RequestInfo()
	if err != nil {
		return err
	}
	role, sets := info.Body["role"]
	if !sets || e.Record.Original().GetString("role") != "lead" || role == "lead" {
		return e.Next()
	}
	if e.Auth != nil && e.Auth.Id == e.Record.Id {
		return e.BadRequestError("no puedes quitarte el rol de lead global a ti mismo: pídeselo a otro lead", nil)
	}
	n, err := liveLeads(e.App)
	if err != nil {
		return err
	}
	if n <= 1 {
		return e.BadRequestError("es el último lead global: nombra a otro antes de quitarle el rol", nil)
	}
	return e.Next()
}

func requireGlobalLead(e *core.RequestEvent) error {
	if !isGlobalLead(e.Auth) {
		return e.ForbiddenError("sólo el lead global gestiona a las personas", nil)
	}
	return nil
}

// createPerson da de alta a una persona, verificada: es alguien por quien el
// lead responde, igual que con `just person` desde la terminal.
func createPerson(e *core.RequestEvent) error {
	if err := requireGlobalLead(e); err != nil {
		return err
	}
	var body struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("cuerpo ilegible", err)
	}
	email := strings.TrimSpace(strings.ToLower(body.Email))
	if email == "" || len(body.Password) < 8 {
		return e.BadRequestError("hace falta un correo y una contraseña de al menos 8 caracteres", nil)
	}
	role := body.Role
	if role != "lead" {
		role = "member"
	}
	if _, err := e.App.FindAuthRecordByEmail("users", email); err == nil {
		return e.BadRequestError("ya hay una cuenta con ese correo (si está borrada, restáurala)", nil)
	}
	col, err := e.App.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.SetPassword(body.Password)
	r.SetVerified(true)
	// Nombrable, como cualquier persona nueva (ver `nameable`): sin el correo
	// visible no se la puede elegir para invitarla ni ponerla a cargo.
	r.SetEmailVisibility(true)
	r.Set("display_name", strings.TrimSpace(body.Name))
	r.Set("role", role)
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, personJSON(r))
}

func personJSON(r *core.Record) map[string]any {
	return map[string]any{
		"id":           r.Id,
		"email":        r.Email(),
		"display_name": r.GetString("display_name"),
		"role":         r.GetString("role"),
		"deleted_at":   r.GetString("deleted_at"),
	}
}

// softDeletePerson marca a la persona y la saca de todas partes a la vez: se
// rota su `tokenKey` —ninguna sesión abierta sobrevive— y sus tokens de agente
// dejan de valer por el middleware. Sus membresías y lo que escribió se quedan.
func softDeletePerson(e *core.RequestEvent) error {
	if err := requireGlobalLead(e); err != nil {
		return err
	}
	r, err := e.App.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("", err)
	}
	if r.Id == e.Auth.Id {
		return e.BadRequestError("no puedes borrarte a ti mismo", nil)
	}
	if !r.GetDateTime("deleted_at").IsZero() {
		return e.JSON(http.StatusOK, personJSON(r))
	}
	if r.GetString("role") == "lead" {
		n, err := liveLeads(e.App)
		if err != nil {
			return err
		}
		if n <= 1 {
			return e.BadRequestError("es el último lead global: nombra a otro antes de borrarlo", nil)
		}
	}
	stamp, _ := types.ParseDateTime(time.Now())
	r.Set("deleted_at", stamp)
	r.RefreshTokenKey()
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, personJSON(r))
}

// restorePerson vacía la marca. Vuelve con sus membresías y sus tokens tal como
// estaban; tiene que iniciar sesión otra vez, porque las sesiones murieron al
// borrarla.
func restorePerson(e *core.RequestEvent) error {
	if err := requireGlobalLead(e); err != nil {
		return err
	}
	r, err := e.App.FindRecordById("users", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("", err)
	}
	r.Set("deleted_at", types.DateTime{})
	if err := e.App.Save(r); err != nil {
		return e.BadRequestError(err.Error(), err)
	}
	return e.JSON(http.StatusOK, personJSON(r))
}
