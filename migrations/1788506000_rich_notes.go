package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El inbox y el plan escriben con el mismo motor que los threads — imágenes
// incluidas — y cada uno guarda lo suyo donde puede.
//
// DOS campos nuevos y una decisión sobre cada uno.
//
// `bubbles.brief`: lo que el lead escribe al planear una burbuja, en markdown,
// con diagramas e imágenes. No es el `outcome`. El outcome es el contrato —una
// frase de 500 caracteres, «qué es cierto cuando esto esté hecho»— y convertirlo
// en el cuaderno del plan lo habría estirado hasta que dejara de ser una frase y
// de poder leerse de un vistazo en el board. El brief es lo largo; el outcome
// sigue siendo lo que se cumple o no.
//
// En la base y no en el árbol de markdown: el árbol tiene sitio para threads,
// docs, README y assets, y una burbuja no es ninguno. Sus IMÁGENES sí van al
// árbol, a `assets/` del workspace, como las de un thread — la burbuja tiene
// workspace, así que tiene dónde.
//
// `inbox_items.files` y `inbox_items.bubble`: una nota NO tiene workspace —no
// tenerlo es lo que la hace una nota— así que sus imágenes no pueden ir a ningún
// `assets/`. Van como archivos del propio registro, igual que las imágenes del
// inventario, y la nota las cita por su URL. `bubble` es a dónde fue a parar al
// triarse: antes una nota promovida se BORRABA, y con ella se iban su cuerpo y
// sus imágenes — ahora queda, apuntando a lo que se convirtió.
func init() {
	m.Register(func(app core.App) error {
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return err
		}
		b.Fields.Add(&core.TextField{Name: "brief", Max: 20000})
		if err := app.Save(b); err != nil {
			return err
		}

		in, err := app.FindCollectionByNameOrId("inbox_items")
		if err != nil {
			return err
		}
		// Sin SVG. Un SVG es un documento que puede ejecutar código, y servido
		// desde nuestro propio origen lo ejecutaría con la sesión de quien lo
		// mira. La ruta de `assets/` de los threads lo neutraliza con una
		// política de seguridad propia; ésta es la de archivos de PocketBase, y
		// no vale la pena apostar a qué cabeceras pone. Una captura de pantalla
		// es PNG o JPEG, que es lo que de verdad se pega aquí.
		in.Fields.Add(&core.FileField{
			Name: "files", MaxSelect: 50, MaxSize: 10 << 20,
			MimeTypes: []string{"image/png", "image/jpeg", "image/webp", "image/gif", "image/avif"},
		})
		in.Fields.Add(&core.RelationField{
			Name: "bubble", CollectionId: b.Id, MaxSelect: 1,
		})
		return app.Save(in)
	}, func(app core.App) error {
		if in, err := app.FindCollectionByNameOrId("inbox_items"); err == nil {
			in.Fields.RemoveByName("files")
			in.Fields.RemoveByName("bubble")
			if err := app.Save(in); err != nil {
				return err
			}
		}
		b, err := app.FindCollectionByNameOrId("bubbles")
		if err != nil {
			return nil
		}
		b.Fields.RemoveByName("brief")
		return app.Save(b)
	})
}
