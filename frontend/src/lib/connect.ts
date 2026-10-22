// El prompt que configura al agente de quien lo pega.
//
// El TEXTO ya no vive aquí: vive en `prompts/connect.md`, embebido en el
// binario y servido en `/api/connect`. Estaba escrito como una cadena de
// plantilla dentro de este archivo, que es exactamente la forma en la que la
// prosa deja de editarse — escapada dentro de código, invisible en un diff de
// los prompts, y imposible de afinar sin tocar el cliente. Es el mismo argumento
// que el paquete `prompts` ya había hecho para la guía.
//
// Lo que se queda aquí son los dos huecos. La URL la sabe el navegador con
// certeza —es por donde llegó—; el servidor tendría que deducirla del `Host` y
// de lo que ponga el proxy de enfrente, y un prompt con la URL equivocada
// configura un servidor que no existe. Y el token es la sesión de quien mira: no
// tiene por qué volver a viajar dentro de una respuesta.
//
// La versión (`{{version}}`) sí la pone el servidor: es la huella de la sección
// que se instala, y la manda también en `X-Bubble-Connect-Version` para los
// comandos por harness.
let template = '';
let version = '';

async function load() {
  if (template) return;
  const r = await fetch('/api/connect');
  if (!r.ok) throw new Error('no se pudo leer el prompt de conexión');
  template = await r.text();
  version = r.headers.get('X-Bubble-Connect-Version') ?? '';
}

/** El prompt listo para pegar. Lleva el token dentro, y por eso el documento
 *  dice explícitamente cómo tratarlo y le pide al asistente configurarse SÓLO a
 *  sí mismo: un agente entusiasta editando la configuración de otros tres es la
 *  forma más rápida de dejar credenciales tiradas por el disco. */
export async function setupPrompt(url: string, token: string): Promise<string> {
  await load();
  return template.replaceAll('{{url}}', url).replaceAll('{{token}}', token);
}

/** La versión vigente de la sección; vacía con un servidor anterior a esto. */
export async function connectVersion(): Promise<string> {
  await load();
  return version;
}

export const CONNECT_HEADER = 'X-Bubble-Connect';

/** Cómo se agrega el servidor a mano en cada harness, según su documentación
 *  (revisada 2026-10): Claude Code y Codex tienen un comando; Antigravity no,
 *  se edita su `mcp_config.json`. El nombre del servidor es `bubble-work`. */
export type Harness = { id: string; name: string; how: string; code: string; lang: 'sh' | 'json' };
export const SERVER_NAME = 'bubble-work';
/** `version` agrega la cabecera X-Bubble-Connect, con la que Ajustes → Tokens
 *  sabe con qué sección se instaló cada agente. Codex no la acepta en `mcp add`
 *  (sólo en su config.toml, `http_headers`), así que va en su explicación. */
export function harnessCommands(url: string, token: string, version = ''): Harness[] {
  const extra = version ? ` \\\n  --header "${CONNECT_HEADER}: ${version}"` : '';
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (version) headers[CONNECT_HEADER] = version;
  return [
    {
      id: 'claude',
      name: 'Claude Code',
      how: 'En la terminal. --scope user lo deja en todos tus proyectos; sin él, sólo en el actual.',
      lang: 'sh',
      code: `claude mcp add --scope user --transport http ${SERVER_NAME} ${url} \\\n  --header "Authorization: Bearer ${token}"${extra}`,
    },
    {
      id: 'codex',
      name: 'Codex',
      how:
        'En la terminal. Codex lee el token de una variable de entorno: deja el export en tu ~/.zshrc (o ~/.bashrc) para que exista en cada sesión.' +
        (version
          ? ` Para que Bubble sepa con qué versión lo instalaste, agrega en ~/.codex/config.toml, bajo [mcp_servers.${SERVER_NAME}]: http_headers = { "${CONNECT_HEADER}" = "${version}" }`
          : ''),
      lang: 'sh',
      code: `export BUBBLE_WORK_TOKEN="${token}"\ncodex mcp add ${SERVER_NAME} --url ${url} --bearer-token-env-var BUBBLE_WORK_TOKEN`,
    },
    {
      id: 'antigravity',
      name: 'Antigravity',
      how: 'No tiene comando: agrega esta entrada a mcpServers en ~/.gemini/config/mcp_config.json (en el IDE: Manage MCP Servers → View raw config), o en .agents/mcp_config.json para un solo proyecto.',
      lang: 'json',
      code: JSON.stringify({ mcpServers: { [SERVER_NAME]: { serverUrl: url, headers } } }, null, 2),
    },
  ];
}

/** Para un token que ya existe el servidor sólo guarda su huella: el prompt va
 *  con un hueco que dice cuál pegar, en vez de fingir que se puede recuperar. */
export function tokenPlaceholder(prefix: string) {
  return `<TU_TOKEN ${prefix}…>`;
}
export function placeholderNote(name: string, prefix: string) {
  return `Antes de seguir: este prompt no trae el token, porque Bubble Work no lo vuelve a enseñar. Reemplaza ${tokenPlaceholder(prefix)} por el token «${name}» que guardaste al generarlo (empieza con ${prefix}). Si no lo tienes, pídeme que genere uno nuevo.\n\n`;
}
