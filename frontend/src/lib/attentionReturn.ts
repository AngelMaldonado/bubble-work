type Frame = { to: string; from: string };
const memory = new Map<string, Frame[]>();
function valid(path: unknown): path is string { return typeof path === 'string' && path.startsWith('/') && !path.startsWith('//'); }
function read(user: string): Frame[] {
  try {
    const raw = sessionStorage.getItem(`bubble.attention.return.${user}`);
    if (raw === null) return memory.get(user) ?? [];
    const rows: unknown = JSON.parse(raw);
    if (Array.isArray(rows)) return rows.filter((r): r is Frame => !!r && valid(r.to) && valid(r.from));
  } catch { /* browser storage unavailable */ }
  return memory.get(user) ?? [];
}
function save(user: string, rows: Frame[]) {
  memory.set(user, rows);
  try { sessionStorage.setItem(`bubble.attention.return.${user}`, JSON.stringify(rows)); } catch { /* keep the session's in-memory fallback */ }
}
export function rememberAttentionReturn(user: string, from: string, to: string) {
  if (!valid(from) || !valid(to) || from === to) return;
  let rows = read(user);
  if (rows.at(-1)?.to !== from) rows = [];
  save(user, [...rows, { from, to }].slice(-20));
}
export function takeAttentionReturn(user: string, path: string): string {
  const rows = read(user);
  const frame = rows.at(-1);
  if (!frame || frame.to !== path) { save(user, []); return ''; }
  save(user, rows.slice(0, -1));
  return frame.from;
}
