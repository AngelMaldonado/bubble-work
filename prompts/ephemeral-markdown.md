# Documentos Markdown efímeros

Un `.md` temporal que publicas para que la persona lo lea ahora en Bubble (un borrador, una propuesta, un informe) sin abrir un hilo ni escribir en la wiki. Bubble no ejecuta ningún modelo: tú publicas desde tu harness.

- Ya lo abriste: `artifact_create(kind: "markdown", …)` te dio un `id` y una `url`. Dale la url a la persona.
- Publica con `artifact_append(id, entries=[{type: "document", data: {filename: "propuesta.md", markdown: "texto completo", base: 0}}])`.
- Para reemplazarlo, llama primero a `artifact_read` y usa `document.seq` como `base`. Cada publicación es una revisión completa e inmutable. Si la base cambió, relee y concilia: nunca sobrescribas a ciegas.
- `artifact_read` devuelve el documento vigente aunque `since` excluya su entrada del historial. Sólo tú publicas; la persona lee en Bubble. Sus casillas son ilustrativas, no editables.
- Sigue la guía de Markdown del departamento. No incluyas secretos: el creador y los leads globales pueden leerlo.
- Es efímero: caduca con la misma retención que los interrogatorios, salvo que alguien lo fije. No produce calor ni commits. Su nota del inbox permanece.
- Si el resultado merece vivir, escríbelo primero como documento independiente en su hilo o módulo, y después usa `artifact_handoff` con esos destinos. No hagas depender trabajo duradero de un enlace que caducará.
- Un `.md` no espera respuestas: no arranques un watcher. Para conversar con preguntas estructuradas, abre un interrogatorio (`kind: "grill"`).
