// The band alphabet, in one place.
//
// It was copied into five components, which is how an emoji ends up meaning two
// things. The lifecycle is a signal the reader learns once; it has to be spelled
// the same everywhere it appears.
//
// Four bands, and they are v0's (`LEVELS` in its `types.ts`) minus 👀 — there is
// no "reviewed" here. Warm and Cooling used to sit between 🔥 and 😴 and were a
// gradient nobody acted on: "produced last cycle" and "produced neither cycle"
// lead to the same morning. Every band left names a different ACTION.
//
//   🔥  producing — leave it alone
//   😴  quiet, with somebody accountable — a person to ask
//   🪦  quiet, with nobody accountable — a decision to make
//   🏆  finished. An ACHIEVEMENT, not a tick: ✓ says the row was processed.
//
// Closed sits at the BOTTOM of the board, where v0 put it: what is done is worth
// seeing and is never what you look at first.
import type { Lifecycle } from './api';

const glyph: Record<Lifecycle, string> = {
  hot: '🔥', dormant: '😴', rip: '🪦', closed: '🏆',
};
const name: Record<Lifecycle, string> = {
  hot: 'Caliente', dormant: 'Dormido', rip: 'Abandonado', closed: 'Terminado',
};

export function bandFace(life: Lifecycle): string {
  return glyph[life] ?? '•';
}

export function bandName(life: Lifecycle): string {
  return name[life] ?? '—';
}

/** Buoyancy order — hottest first, finished last. */
export const bandOrder: Lifecycle[] = ['hot', 'dormant', 'rip', 'closed'];
