// Las funciones que se ven: las del departamento (`features`), matizadas por lo
// que decide cada equipo (`team_features`). El valor de un equipo, si lo dice;
// si no, el del departamento. Mi espacio no es de un equipo: siempre global.
import type { Features, Team, TeamFeatureKey } from './api';

export const TEAM_FEATURES: { key: TeamFeatureKey; label: string; icon: string }[] = [
  { key: 'sequence', label: 'Secuencia', icon: 'compass' },
  { key: 'roadmap', label: 'Roadmap', icon: 'map' },
];

/** ¿Encendida para este equipo? Sin equipo, la del departamento. */
export function teamFeatureOn(global: Features, team: Pick<Team, 'features'> | undefined, key: TeamFeatureKey): boolean {
  const v = team?.features?.[key];
  if (v === 'on') return true;
  if (v === 'off') return false;
  return !!global[key];
}

/** Los ids de los equipos que tienen una función encendida. */
export function teamsWith(global: Features, teams: (Pick<Team, 'features'> & { id: string })[], key: TeamFeatureKey): string[] {
  return teams.filter((t) => teamFeatureOn(global, t, key)).map((t) => t.id);
}

/** Las funciones como las ve quien mira: las del equipo actual o, en «Todos
 *  los equipos», encendida si algún equipo de la vista la tiene. */
export function effectiveFeatures(global: Features, teams: Pick<Team, 'features'>[]): Features {
  const out: Features = { ...global };
  for (const { key } of TEAM_FEATURES) {
    out[key] = teams.length ? teams.some((t) => teamFeatureOn(global, t, key)) : !!global[key];
  }
  return out;
}
