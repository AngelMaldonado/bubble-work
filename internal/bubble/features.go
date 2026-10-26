package bubble

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// Funciones por equipo (`team_features`) sobre las del departamento
// (`features`): el valor de un equipo, si lo dice; si no, el global.
//
// Sólo secuencia y roadmap: son de cada equipo. Mi espacio es de cada persona y
// se queda global.

// TeamFeatureKeys son las funciones que un equipo puede decidir por su cuenta.
var TeamFeatureKeys = []string{"sequence", "roadmap"}

func isTeamFeature(key string) bool {
	for _, k := range TeamFeatureKeys {
		if k == key {
			return true
		}
	}
	return false
}

func globalFeature(app core.App, key string) bool {
	f, err := app.FindFirstRecordByFilter("features", "id != ''")
	return err == nil && f.GetBool(key)
}

func teamFeatureRow(app core.App, team string) *core.Record {
	if team == "" {
		return nil
	}
	r, err := app.FindFirstRecordByData("team_features", "team", team)
	if err != nil {
		return nil
	}
	return r
}

// TeamFeatures es lo que un equipo dice de cada función: "on", "off" o "" (hereda).
func TeamFeatures(app core.App, team string) map[string]string {
	out := make(map[string]string, len(TeamFeatureKeys))
	r := teamFeatureRow(app, team)
	for _, k := range TeamFeatureKeys {
		if r != nil {
			out[k] = r.GetString(k)
		} else {
			out[k] = ""
		}
	}
	return out
}

// FeatureOn dice si una función está encendida para un equipo. Sin equipo, o
// si el equipo no la decide, manda la del departamento.
func FeatureOn(app core.App, team, key string) bool {
	if isTeamFeature(key) {
		if r := teamFeatureRow(app, team); r != nil {
			switch r.GetString(key) {
			case "on":
				return true
			case "off":
				return false
			}
		}
	}
	return globalFeature(app, key)
}

// SetTeamFeature fija lo que un equipo dice de una función. Sólo quien lidera
// el equipo; `value` es "on", "off" o "" para volver a heredar.
func SetTeamFeature(app core.App, auth *core.Record, team, key, value string) (map[string]string, error) {
	if !isPersonAuth(auth) && !IsSuperuser(auth) {
		return nil, ErrNotAPerson
	}
	if !LeadsTeam(app, auth, team) {
		return nil, errNotTeamLead
	}
	if !isTeamFeature(key) {
		return nil, errUnknownFeature
	}
	if value != "" && value != "on" && value != "off" {
		return nil, errFeatureValue
	}
	r := teamFeatureRow(app, team)
	if r == nil {
		col, err := app.FindCollectionByNameOrId("team_features")
		if err != nil {
			return nil, err
		}
		r = core.NewRecord(col)
		r.Set("team", team)
	}
	r.Set(key, value)
	if err := app.Save(r); err != nil {
		return nil, err
	}
	return TeamFeatures(app, team), nil
}

type featureError struct {
	msg    string
	status int
}

func (e *featureError) Error() string { return e.msg }

var (
	errNotTeamLead    = &featureError{"sólo un lead del equipo decide sus funciones", http.StatusForbidden}
	errUnknownFeature = &featureError{"un equipo decide la secuencia o el roadmap; Mi espacio es del departamento", http.StatusBadRequest}
	errFeatureValue   = &featureError{"el valor es on, off o vacío (hereda el del departamento)", http.StatusBadRequest}
)
