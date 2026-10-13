Agrega el servidor MCP de Bubble Work a tu propia configuración.

Hazlo SÓLO PARA TI — el asistente con el que estoy hablando ahora. No configures ningún otro asistente, editor, CLI ni herramienta, y no edites archivos de configuración que pertenezcan a otro. Si no tienes claro qué cliente eres, pregúntame en vez de tocar varios.

Protocolo: HTTP (streamable http)
URL: {{url}}
Autenticación: envía la cabecera  Authorization: Bearer {{token}}

Averigua cuál es el archivo de configuración que tú mismo lees y sigue tu propio esquema para un servidor HTTP con cabeceras personalizadas, en vez de adivinar la ruta.

Luego confirma que funcionó listando las tools. Deberías ver, entre otras, `agents_md`, `sequence_next`, `project_list`, `document_read`, `document_edit` y `thread_create`. Si falla, dime el error exacto en vez de reintentar a ciegas.

Cuando el MCP responda, llama a `agents_md` y léelo completo: es la guía de Bubble Work (entidades, cómo se escribe, todas las tools por categoría) más la forma de trabajar del departamento, que mantiene el lead. Se lee en vivo, así que no lo copies a ningún archivo: un cambio del lead llega a la siguiente sesión sin reinstalar nada.

Después añade esta sección a tus instrucciones de este proyecto, el archivo que ya lees en cada sesión (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`, `.github/copilot-instructions.md`, el que corresponda a lo que tú eres). Agrégala al final, sin reescribir lo que ya hay. Si ya existe una sección «Bubble Work», reemplázala COMPLETA por esta: las versiones viejas nombran tools que ya no existen. Antes de guardarla, sustituye `<SLUG>` por el proyecto que corresponde a este repositorio: `project_list` te dice cuáles hay, y si ninguno es obvio, pregúntame cuál.

```markdown
## Bubble Work

El trabajo de este proyecto se planea y se anota en Bubble Work, por MCP. El proyecto es `<SLUG>`.

**Al empezar cada sesión, antes de cualquier otra cosa:** si la forma de trabajar del departamento no está ya en tu contexto, llama a `agents_md` y síguela. Es la guía de Bubble Work y la forma de trabajar del departamento; se lee en vivo y manda sobre lo que recuerdes y sobre esta sección.

**Antes de crear, busca.** Lo normal es que ya exista un hilo a medias para lo que te piden: trabaja en ese. Lo que encuentres, decidas o dejes a medias va en el documento de SU hilo.
```

Esa sección es breve a propósito: sin ella, cada conversación vuelve a inventar dónde se anota el trabajo y abre un hilo nuevo al lado del que ya estaba a medias. Si ya estaba instalada, volver a pegar este prompt la actualiza.

Ese token es la llave de este agente y actúa en mi nombre: quien lo tenga trabaja como yo, y lo que escribas queda firmado con mi nombre. Ponlo en el archivo de configuración, no me lo repitas y no lo escribas en ningún otro lado. Si se filtra, lo revoco desde Ajustes → Tokens.
