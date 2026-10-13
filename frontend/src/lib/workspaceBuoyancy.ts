import type { Board, Lifecycle } from './api';
import { bandOrder } from './bands';
import { when } from './when';
export function workspaceBuoyancy(board: Pick<Board, 'bubbles'>): Record<string, { life: Lifecycle; at: string }> {
  const out: Record<string, { life: Lifecycle; at: string }> = {};
  for (const b of board.bubbles) {
    const row = out[b.workspace] ??= { life: 'closed', at: '' };
    if (bandOrder.indexOf(b.heat.lifecycle) < bandOrder.indexOf(row.life)) row.life = b.heat.lifecycle;
    if (b.warm_at && (!row.at || when(b.warm_at) > when(row.at))) row.at = b.warm_at;
  }
  return out;
}
