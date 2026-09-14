package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// El cuerpo de una nota y el brief de una burbuja caben lo que cabe un
// documento.
//
// 20000 caracteres se eligió pensando en PROSA, y en prosa es mucho. Pero estos
// campos escriben con el mismo motor que los threads, y ese motor dibuja: un
// bloque ```excalidraw es la escena entera en JSON, y un diagrama de una docena
// de cajas y flechas ya pasa de 30000 caracteres antes de la primera frase. Con
// el tope viejo, pegar un diagrama —justo lo que el motor compartido prometía—
// era una escritura rechazada.
//
// El documento de un thread no tiene tope propio; vive en el árbol y lo limita
// sólo el tamaño de la petición. Aquí el tope se queda —un campo de texto sin
// Max en PocketBase NO es ilimitado, es 5000— pero en una cifra que un diagrama
// no alcanza por serlo: 2 millones de caracteres. Lo que pase de eso no es una
// nota, y las imágenes van como archivos, no dentro del texto.
const roomyText = 2_000_000

func init() {
	m.Register(func(app core.App) error {
		return widen(app, roomyText)
	}, func(app core.App) error {
		// Bajar el tope no recorta lo que ya se escribió: un registro más largo
		// sólo dejaría de poder guardarse. Por eso el camino de vuelta es el
		// tope viejo y nada más.
		return widen(app, 20000)
	})
}

func widen(app core.App, max int) error {
	for _, f := range []struct{ collection, field string }{
		{"inbox_items", "body"},
		{"bubbles", "brief"},
	} {
		c, err := app.FindCollectionByNameOrId(f.collection)
		if err != nil {
			return err
		}
		field, ok := c.Fields.GetByName(f.field).(*core.TextField)
		if !ok {
			continue
		}
		field.Max = max
		if err := app.Save(c); err != nil {
			return err
		}
	}
	return nil
}
