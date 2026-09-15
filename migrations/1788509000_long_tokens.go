package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El token de una persona dura tres años, no cinco días.
//
// El agente se conecta con el token de la sesión de quien copió el prompt
// (`/mcp` es `RequireAuth`, y el agente ES esa persona). El navegador renueva el
// suyo solo con `auth-refresh`; la copia pegada en la configuración del agente no
// se renueva nunca. Con el valor de PocketBase —432000 s, cinco días— el MCP
// dejaba de responder a los cinco días con un 401, y el cliente no tenía cómo
// recuperarse: con `Authorization` en la cabecera no hay OAuth al que caer.
//
// Tres años es el tope que PocketBase acepta para este ajuste (94670856 s). Sube
// también la sesión del navegador, que ya se renovaba sola y ahora simplemente
// no caduca en la práctica. Revocar sigue siendo lo mismo que antes: cambiar la
// contraseña rota el `tokenKey` y mata todos los tokens de esa persona.
//
// Un token ya emitido lleva su caducidad dentro y no cambia: hay que volver a
// copiar el prompt para que el agente tenga uno de tres años.
const longToken = 94670856 // ~3 años, el máximo de PocketBase

func init() {
	m.Register(func(app core.App) error {
		return tokenLasts(app, longToken)
	}, func(app core.App) error {
		return tokenLasts(app, 432000)
	})
}

func tokenLasts(app core.App, seconds int64) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.AuthToken.Duration = seconds
	return app.Save(users)
}
