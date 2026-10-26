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

// deletedPersonKeep: cuánto se guardan los datos personales de una persona
// borrada. Dentro de ese plazo, restaurarla la deja como estaba; después, el
// job los purga y queda sólo la cuenta, como firma de lo que escribió.
const deletedPersonKeep = 30 * 24 * time.Hour

// personalData: lo que es DE la persona y no del trabajo — se va con la purga.
// Lo que escribió (hilos, comentarios, eventos, artefactos, versiones) se
// queda: es la historia del departamento, y lleva su nombre.
var personalData = []struct{ collection, field string }{
	{"agent_tokens", "owner"},
	{"prefs", "user"},
	{"presence", "user"},
	{"personal_notes", "owner"},
	{"personal_assets", "owner"},
	{"inventory_access", "user"},
	{"memberships", "user"},
	{"team_members", "user"},
}

// sweepDeletedPeople purga a quien lleva más de `deletedPersonKeep` borrada y
// no se ha purgado desde que la borraron. Devuelve a cuántas.
func sweepDeletedPeople(app core.App, now time.Time) int {
	cutoff := now.Add(-deletedPersonKeep).UTC().Format("2006-01-02 15:04:05.000Z")
	rows, err := app.FindRecordsByFilter("users",
		"deleted_at != '' && deleted_at < {:c} && (purged_at = '' || purged_at < deleted_at)", "", 0, 0,
		dbx.Params{"c": cutoff})
	if err != nil {
		return 0
	}
	n := 0
	for _, u := range rows {
		if err := purgePerson(app, u, now); err != nil {
			app.Logger().Error("no se pudo purgar a una persona borrada", "user", u.Id, "error", err)
			continue
		}
		n++
	}
	return n
}

// purgePerson borra sus datos personales, en una transacción: o todo o nada.
// La cuenta se queda (nombre y correo), así lo que escribió sigue firmado.
func purgePerson(app core.App, u *core.Record, now time.Time) error {
	return app.RunInTransaction(func(tx core.App) error {
		for _, d := range personalData {
			rows, err := tx.FindRecordsByFilter(d.collection, d.field+" = {:u}", "", 0, 0, dbx.Params{"u": u.Id})
			if err != nil {
				continue // la colección no existe en esta instancia
			}
			for _, r := range rows {
				if err := tx.Delete(r); err != nil {
					return err
				}
			}
		}
		// Su equipo personal se va con ella, salvo que tenga proyectos: esos
		// son trabajo, y borrarlos sería borrar historia.
		teams, _ := tx.FindRecordsByFilter("teams", "personal = true && owner = {:u}", "", 0, 0, dbx.Params{"u": u.Id})
		for _, t := range teams {
			if ws, _ := tx.FindRecordsByFilter("workspaces", "team = {:t}", "", 1, 0, dbx.Params{"t": t.Id}); len(ws) > 0 {
				continue
			}
			if err := tx.Delete(t); err != nil {
				return err
			}
		}
		stamp, _ := types.ParseDateTime(now)
		u.Set("avatar", nil)
		u.Set("purged_at", stamp)
		return tx.Save(u)
	})
}

func registerPeople(app core.App) {
	app.OnRecordUpdateRequest("users").BindFunc(keepAGlobalLead)
	app.Cron().MustAdd("bubblePeoplePurge", "41 3 * * *", func() {
		if n := sweepDeletedPeople(app, time.Now()); n > 0 {
			app.Logger().Info("deleted people purged", "count", n)
		}
	})

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
		"purged_at":    r.GetString("purged_at"),
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
