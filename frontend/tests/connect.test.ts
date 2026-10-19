import { expect, test } from 'bun:test';
import { harnessCommands, placeholderNote, tokenPlaceholder } from '../src/lib/connect';

const url = 'https://bubble.example/mcp';
const tok = 'bw_abc';

test('los tres harness llevan la URL y el token', () => {
  const hs = harnessCommands(url, tok);
  expect(hs.map((h) => h.id)).toEqual(['claude', 'codex', 'antigravity']);
  for (const h of hs) {
    expect(h.code).toContain(url);
    expect(h.code).toContain(tok);
  }
});

test('Claude Code: comando http con la cabecera', () => {
  const c = harnessCommands(url, tok)[0].code;
  expect(c).toContain('claude mcp add --scope user --transport http bubble-work https://bubble.example/mcp');
  expect(c).toContain('--header "Authorization: Bearer bw_abc"');
});

test('Codex: el token por variable de entorno', () => {
  const c = harnessCommands(url, tok)[1].code;
  expect(c).toContain('export BUBBLE_WORK_TOKEN="bw_abc"');
  expect(c).toContain('codex mcp add bubble-work --url https://bubble.example/mcp --bearer-token-env-var BUBBLE_WORK_TOKEN');
});

test('Antigravity: JSON válido con serverUrl y headers', () => {
  const j = JSON.parse(harnessCommands(url, tok)[2].code);
  expect(j.mcpServers['bubble-work']).toEqual({ serverUrl: url, headers: { Authorization: 'Bearer bw_abc' } });
});

test('el hueco del token dice cuál pegar', () => {
  expect(tokenPlaceholder('bw_WzQ6PU')).toBe('<TU_TOKEN bw_WzQ6PU…>');
  expect(placeholderNote('Local', 'bw_WzQ6PU')).toContain('«Local»');
});
