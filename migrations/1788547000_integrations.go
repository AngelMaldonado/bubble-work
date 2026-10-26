package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Integraciones: otro sistema (cuby.work, el primero) deja trabajo aquí y se
// entera de cuándo terminó, sin un agente de por medio.
//
//   - `external_refs` dice qué es aquí una cosa de allá: la tarea X de cuby es
//     el hilo Y. Es lo que vuelve idempotente «asegura el hilo de la tarea X»:
//     si la respuesta se perdió y cuby reintenta, encuentra el que ya creó en
//     vez de abrir otro. Una tabla aparte y no un campo en `threads` y
//     `bubbles`: el modelo no aprende nada de una integración, y una segunda no
//     pide otra columna.
//   - `webhooks` son los destinos que configura el lead global, con su secreto
//     (oculto: se enseña una vez) y los eventos que escuchan.
//   - `webhook_deliveries` es la cola: un aviso por evento y destino, con sus
//     intentos. El webhook de los semáforos no reintenta porque su latido
//     corrige; un «terminado» perdido no se corrige solo.
//
// Ninguna lleva reglas de API: se leen y escriben por /api/integrations y
// /api/webhooks, que deciden quién puede.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		workspaces, err := app.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}

		refs := core.NewBaseCollection("external_refs")
		refs.Fields.Add(&core.TextField{Name: "source", Required: true, Max: 40})
		refs.Fields.Add(&core.TextField{Name: "ref", Required: true, Max: 200})
		refs.Fields.Add(&core.SelectField{Name: "kind", Required: true, MaxSelect: 1,
			Values: []string{"project", "bubble", "thread"}})
		refs.Fields.Add(&core.TextField{Name: "target", Required: true, Max: 40})
		refs.Fields.Add(&core.RelationField{Name: "workspace", CollectionId: workspaces.Id,
			MaxSelect: 1, CascadeDelete: true})
		refs.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		refs.AddIndex("idx_external_refs_key", true, "source, kind, ref", "")
		refs.AddIndex("idx_external_refs_target", false, "target", "")
		if err := app.Save(refs); err != nil {
			return err
		}

		hooks := core.NewBaseCollection("webhooks")
		hooks.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 80})
		hooks.Fields.Add(&core.URLField{Name: "url", Required: true})
		hooks.Fields.Add(&core.TextField{Name: "secret", Hidden: true, Max: 128})
		hooks.Fields.Add(&core.JSONField{Name: "events", MaxSize: 4000})
		hooks.Fields.Add(&core.BoolField{Name: "active"})
		// Lo que escribe esta persona no se avisa a este destino: la cuenta
		// con la que la integración escribe aquí, para que no se oiga a sí misma.
		hooks.Fields.Add(&core.RelationField{Name: "skip_actor", CollectionId: users.Id, MaxSelect: 1})
		hooks.Fields.Add(&core.TextField{Name: "last_status", Max: 300})
		hooks.Fields.Add(&core.DateField{Name: "last_at"})
		hooks.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		hooks.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		if err := app.Save(hooks); err != nil {
			return err
		}

		events, err := app.FindCollectionByNameOrId("events")
		if err != nil {
			return err
		}
		deliveries := core.NewBaseCollection("webhook_deliveries")
		deliveries.Fields.Add(&core.RelationField{Name: "webhook", CollectionId: hooks.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true})
		deliveries.Fields.Add(&core.RelationField{Name: "event", CollectionId: events.Id,
			Required: true, MaxSelect: 1, CascadeDelete: true})
		deliveries.Fields.Add(&core.TextField{Name: "type", Required: true, Max: 60})
		deliveries.Fields.Add(&core.SelectField{Name: "status", Required: true, MaxSelect: 1,
			Values: []string{"pending", "sent", "dead"}})
		deliveries.Fields.Add(&core.NumberField{Name: "attempts", OnlyInt: true})
		deliveries.Fields.Add(&core.DateField{Name: "next_at"})
		deliveries.Fields.Add(&core.TextField{Name: "last_error", Max: 300})
		deliveries.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		deliveries.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
		deliveries.AddIndex("idx_webhook_deliveries_key", true, "webhook, event", "")
		deliveries.AddIndex("idx_webhook_deliveries_due", false, "status, next_at", "")
		return app.Save(deliveries)
	}, func(app core.App) error {
		for _, name := range []string{"webhook_deliveries", "webhooks", "external_refs"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
