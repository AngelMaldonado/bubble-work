// El prompt que configura al agente de quien lo pega.
//
// Portado de v0 (`McpConnect`), sin su ventana: allá era un diálogo con la URL,
// el token enmascarado y tres formas de copiar. Casi todo eso era ceremonia —
// lo que la gente usaba era el prompt, y una ventana para llegar a un botón es
// una ventana de más.
//
// Lleva el token dentro, y por eso dice explícitamente cómo tratarlo, y le pide
// al asistente configurarse SÓLO a sí mismo: un agente entusiasta editando la
// configuración de otros tres es la forma más rápida de dejar credenciales
// tiradas por el disco.
export function setupPrompt(url: string, token: string): string {
  return `Agrega el servidor MCP de Bubble Work a tu propia configuración.

Hazlo SÓLO PARA TI — el asistente con el que estoy hablando ahora. No configures ningún otro asistente, editor, CLI ni herramienta, y no edites archivos de configuración que pertenezcan a otro. Si no tienes claro qué cliente eres, pregúntame en vez de tocar varios.

Protocolo: HTTP (streamable http)
URL: ${url}
Autenticación: envía la cabecera  Authorization: Bearer ${token}

Averigua cuál es el archivo de configuración que tú mismo lees y sigue tu propio esquema para un servidor HTTP con cabeceras personalizadas, en vez de adivinar la ruta.

Luego confirma que funcionó listando las herramientas. Deberías ver guide, board, read, edit, create_thread y timeline entre ellas. Si falla, dime el error exacto en vez de reintentar a ciegas.

Antes de escribir nada en Bubble Work, lee la herramienta \`guide\`: dice qué cuenta como evidencia y qué no, dónde viven los archivos, y cómo funciona una escritura con hash. No valida la forma de un documento a propósito.

Ese token es mi sesión: quien lo tenga trabaja como yo, y lo que escribas queda firmado con mi nombre. Ponlo en el archivo de configuración, no me lo repitas y no lo escribas en ningún otro lado.`;
}
