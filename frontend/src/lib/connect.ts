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
let template = '';

/** El prompt listo para pegar. Lleva el token dentro, y por eso el documento
 *  dice explícitamente cómo tratarlo y le pide al asistente configurarse SÓLO a
 *  sí mismo: un agente entusiasta editando la configuración de otros tres es la
 *  forma más rápida de dejar credenciales tiradas por el disco. */
export async function setupPrompt(url: string, token: string): Promise<string> {
  if (!template) {
    const r = await fetch('/api/connect');
    if (!r.ok) throw new Error('no se pudo leer el prompt de conexión');
    template = await r.text();
  }
  return template.replaceAll('{{url}}', url).replaceAll('{{token}}', token);
}
