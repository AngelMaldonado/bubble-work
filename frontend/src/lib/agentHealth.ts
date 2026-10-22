// ¿Tiene este agente lo vigente? Lo que el servidor VIO, comparado con las
// releases de ahora.
//
// Dos preguntas distintas, con dos arreglos distintos:
//   - la release de `agents_md` que se le entregó: si es vieja, se arregla sola
//     en su siguiente sesión (las `instructions` del servidor se lo dicen);
//   - con qué sección se instaló (la cabecera X-Bubble-Connect): si es vieja,
//     `agents_md` le trae la nueva, y si no la toma, hay que volver a pegar el
//     prompt de conexión.
//
// Esto prueba que el texto LLEGÓ, no que el modelo lo siguiera.
import type { AgentReleases, AgentToken } from './api';

export type ReleaseState = 'current' | 'stale' | 'never';
export type InstallState = 'current' | 'old' | 'unknown';
export type Chip = { key: 'current' | 'stale' | 'never' | 'old'; text: string; hint: string };
export type AgentHealth = {
  release: ReleaseState;
  install: InstallState;
  chips: Chip[];
};

/** Lo que se enseña de una huella: siete caracteres, como un commit. */
export const short = (v?: string) => (v ?? '').slice(0, 7);

/** Sin uso, sin releases (un servidor viejo) o revocado: no hay nada que decir. */
export function agentHealth(t: AgentToken, cur: AgentReleases | null): AgentHealth | null {
  if (!cur || !t.last_used_at || t.revoked_at) return null;

  const want = t.release_role === 'planner' ? cur.planner : cur.operators;
  const release: ReleaseState = !t.release_read ? 'never' : want && t.release_read !== want ? 'stale' : 'current';
  const install: InstallState = !t.connect_version
    ? 'unknown'
    : cur.connect && t.connect_version !== cur.connect
      ? 'old'
      : 'current';

  const chips: Chip[] = [];
  if (release === 'never') {
    chips.push({
      key: 'never',
      text: 'nunca leyó agents_md',
      hint:
        'Usa el MCP pero nunca ha llamado a agents_md: su cliente no entrega las instrucciones del servidor y su sección no lo hizo leerla. Vuelve a pegar el prompt de conexión.',
    });
  } else if (release === 'stale') {
    chips.push({
      key: 'stale',
      text: 'release anterior',
      hint: `Leyó ${short(t.release_read)}; la vigente es ${short(want)}. Se pone al día sola en su siguiente sesión: las instrucciones del servidor se lo dicen.`,
    });
  } else {
    chips.push({ key: 'current', text: 'al día', hint: `Leyó la release vigente de agents_md (${short(want)}).` });
  }
  if (install === 'old') {
    chips.push({
      key: 'old',
      text: 'instalación vieja',
      hint: `Se instaló con la sección ${short(t.connect_version)}; la vigente es ${short(cur.connect)}. agents_md le trae la nueva; si no cambia, vuelve a pegar el prompt de conexión.`,
    });
  }
  return { release, install, chips };
}
