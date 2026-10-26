// Con qué agentes trabaja cada persona, y qué AGENTS.md les llegó.
//
// Sale de sus tokens: el `clientInfo` que mandó cada agente al conectarse y la
// release de `agents_md` que el servidor le entregó (release.go). Es el CLIENTE
// —Claude Code, Codex, Cursor…—, no el modelo: MCP no dice qué modelo hay
// detrás, y adivinarlo sería inventar.
import type { AgentReleases, AgentToken } from './api';

/** Una marca: logo de simple-icons (`/logos/<slug>.svg`) o, si no hay, una
 *  inicial. `hex` sólo en las de color; las negras toman el color del texto,
 *  para que se vean en el tema oscuro. */
export type ClientBrand = { key: string; name: string; slug?: string; hex?: string; mono?: string };

// En orden: la primera que encaja gana («claude-code» antes que «code»).
// Codex (OpenAI) y Antigravity no tienen logo en simple-icons: van con inicial.
const BRANDS: [RegExp, ClientBrand][] = [
  // El logo de Claude y no el de Claude Code (`claudecode`, la mascota
  // pixelada): se reconoce de un vistazo, y Claude Code siempre es Claude.
  [/claude/i, { key: 'claude', name: 'Claude Code', slug: 'claude', hex: 'D97757' }],
  [/codex|openai/i, { key: 'codex', name: 'Codex', mono: 'Cx' }],
  [/antigravity/i, { key: 'antigravity', name: 'Antigravity', mono: 'Ag' }],
  [/gemini/i, { key: 'gemini', name: 'Gemini CLI', slug: 'googlegemini', hex: '8E75B2' }],
  [/cursor/i, { key: 'cursor', name: 'Cursor', slug: 'cursor' }],
  [/copilot|visual studio code|vscode/i, { key: 'copilot', name: 'GitHub Copilot', slug: 'githubcopilot' }],
  [/windsurf|codeium/i, { key: 'windsurf', name: 'Windsurf', slug: 'windsurf' }],
  [/cline/i, { key: 'cline', name: 'Cline', slug: 'cline' }],
  [/\bzed\b/i, { key: 'zed', name: 'Zed', slug: 'zedindustries', hex: '084CCF' }],
  [/^pi\b|pi-coding-agent/i, { key: 'pi', name: 'pi', mono: 'π' }],
];

/** `client` es «nombre versión», como lo guarda el servidor. */
export function brandOf(client: string): ClientBrand & { version: string } {
  const c = client.trim();
  const sp = c.lastIndexOf(' ');
  const version = sp > 0 && /\d/.test(c.slice(sp + 1)) ? c.slice(sp + 1) : '';
  const name = version ? c.slice(0, sp) : c;
  for (const [re, b] of BRANDS) if (re.test(name)) return { ...b, version };
  return { key: `other:${name.toLowerCase()}`, name: name || 'desconocido', mono: (name[0] ?? '?').toUpperCase(), version };
}

export type ClientUse = ClientBrand & { version: string; last: string; tokens: number };
export type PersonAgents = {
  /** sus clientes, del usado más recientemente al más viejo */
  clients: ClientUse[];
  /** la release de `agents_md` más reciente que le llegó a alguno de sus agentes */
  release: { id: string; role: string; at: string; current: boolean } | null;
  /** tiene agentes que usan el MCP y ninguno ha leído `agents_md` */
  neverRead: boolean;
};

const newer = (a?: string, b?: string) => (a ?? '') > (b ?? '');

/** Por persona (`owner`), sólo tokens vivos que ya se usaron. Sin `cur` (un
 *  servidor anterior), `current` es falso: no se afirma lo que no se sabe. */
export function agentsByPerson(tokens: AgentToken[], cur: AgentReleases | null): Record<string, PersonAgents> {
  const out: Record<string, PersonAgents> = {};
  for (const t of tokens) {
    if (t.revoked_at || !t.last_used_at) continue;
    const p = (out[t.owner] ??= { clients: [], release: null, neverRead: false });
    if (t.client) {
      const b = brandOf(t.client);
      const seen = p.clients.find((c) => c.key === b.key);
      if (!seen) p.clients.push({ ...b, last: t.last_used_at, tokens: 1 });
      else {
        seen.tokens++;
        if (newer(t.last_used_at, seen.last)) Object.assign(seen, { version: b.version, last: t.last_used_at });
      }
    }
    if (t.release_read && (!p.release || newer(t.release_read_at, p.release.at))) {
      const role = t.release_role || 'operators';
      const want = role === 'planner' ? cur?.planner : cur?.operators;
      p.release = { id: t.release_read, role, at: t.release_read_at ?? '', current: !!want && want === t.release_read };
    }
  }
  for (const p of Object.values(out)) {
    p.clients.sort((a, b) => (b.last > a.last ? 1 : b.last < a.last ? -1 : 0));
    p.neverRead = !p.release;
  }
  return out;
}
