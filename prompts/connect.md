Agrega el servidor MCP de Bubble Work a tu propia configuración.

Hazlo SÓLO PARA TI — el asistente con el que estoy hablando ahora. No configures ningún otro asistente, editor, CLI ni herramienta, y no edites archivos de configuración que pertenezcan a otro. Si no tienes claro qué cliente eres, pregúntame en vez de tocar varios.

Protocolo: HTTP (streamable http)
URL: {{url}}
Autenticación: envía la cabecera  Authorization: Bearer {{token}}

Averigua cuál es el archivo de configuración que tú mismo lees y sigue tu propio esquema para un servidor HTTP con cabeceras personalizadas, en vez de adivinar la ruta.

Luego confirma que funcionó listando las herramientas. Deberías ver guide, agents_md, board, read, edit, create_thread y timeline entre ellas. Si falla, dime el error exacto en vez de reintentar a ciegas.

Antes de escribir nada en Bubble Work, lee la herramienta `guide`: dice qué cuenta como evidencia y qué no, dónde viven los archivos, y cómo funciona una escritura con hash. No valida la forma de un documento a propósito.

Cuando el MCP responda, llama a la herramienta `house_rules` y añade lo que devuelve a tus instrucciones globales de este proyecto — el archivo que ya lees en cada sesión: `CLAUDE.md`, `AGENTS.md`, `.cursorrules`, `.github/copilot-instructions.md`, el que corresponda a lo que tú eres. Añádelo como una sección más al final; no reescribas lo que ya hay, y si ya existe una sección de Bubble Work, actualízala en vez de duplicarla. Antes de guardarla, sustituye el hueco del workspace por el que corresponde a este proyecto — `workspaces` te dice cuáles hay, y si ninguno es obvio, pregúntame cuál.

Esa sección es breve a propósito: explica el framework y te obliga a leer, al empezar cada sesión, `agents_md` — la forma de trabajar del departamento, que mantiene el lead en el servidor. No copies ese documento a tu archivo: se lee en vivo para que un cambio del lead llegue a la siguiente sesión sin reinstalar nada. Léelo ahora una vez, para confirmar que responde.

Eso es lo que hace que la próxima sesión —la tuya o la de otro— no empiece a ciegas: sin esa sección, cada conversación vuelve a inventar dónde se anota el trabajo y cómo se trabaja. Si la sección ya estaba instalada, volver a pegar este prompt la actualiza.

Ese token es la llave de este agente y actúa en mi nombre: quien lo tenga trabaja como yo, y lo que escribas queda firmado con mi nombre. Ponlo en el archivo de configuración, no me lo repitas y no lo escribas en ningún otro lado. Si se filtra, lo revoco desde Ajustes → Tokens.
