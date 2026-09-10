/**
 * What the four priorities mean, and the map that decides them.
 *
 * NOT in `mock.ts`, where these used to live: they are not invented data. The
 * server derives a thread's priority from impact × urgency with exactly this
 * table (`internal/bubble` — `thread_priority`), and this is the copy a person
 * reads before choosing an impact and an urgency. A legend that disagrees with
 * the rule it explains is worse than no legend, so when one moves, both do.
 */

/** code · name · what it is · what happens */
export const priorityMeaning: [string, string, string, string][] = [
  ['P1', 'Crítica', 'Operación detenida', 'Interrumpe cualquier trabajo'],
  ['P2', 'Alta', 'Impacto fuerte, pero hay workaround', 'Se atiende rápidamente'],
  ['P3', 'Normal', 'Trabajo necesario normal', 'Entra en planeación'],
  ['P4', 'Baja', 'Mejora, nice-to-have', 'Backlog'],
];

/** Impact × urgency, the same square the server computes from. */
export const priorityMap = {
  cols: ['Urgencia alta', 'Media', 'Baja'],
  rows: [
    ['Impacto alto', 'P1', 'P2', 'P3'],
    ['Impacto medio', 'P2', 'P2', 'P3'],
    ['Impacto bajo', 'P3', 'P3', 'P4'],
  ],
};
