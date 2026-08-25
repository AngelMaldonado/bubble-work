<script lang="ts">
  // Connect your agent (docs/journal/MCP-ACCESS.md step 5).
  //
  // The server's own MCP endpoint is authenticated exactly like the REST API:
  // you present your Plane key as a Bearer credential, and the framework's rules
  // are enforced server-side so they cannot be bypassed from any client (§9).
  //
  // The token shown here is the one this browser already holds — nothing is
  // minted and nothing new is disclosed. It is masked anyway, because a
  // credential on a screen is a credential in a screenshot.
  import { getToken } from '../lib/api';
  import { t } from '../lib/i18n.svelte';
  import { store } from '../lib/store.svelte';

  let { onclose }: { onclose: () => void } = $props();

  let revealed = $state(false);
  let copied = $state<string | null>(null);

  const token = $derived(getToken());
  const url = $derived(`${window.location.origin}/mcp`);
  const masked = $derived(
    token.length > 12 ? `${token.slice(0, 6)}${'•'.repeat(18)}${token.slice(-4)}` : '••••••••',
  );

  // The one-liner for Claude Code, and the block for a global config file.
  const cli = $derived(
    `claude mcp add --transport http bubble ${url} \\\n  --header "Authorization: Bearer ${token}"`,
  );
  const json = $derived(
    JSON.stringify(
      {
        mcpServers: {
          bubble: {
            type: 'http',
            url,
            headers: { Authorization: `Bearer ${token}` },
          },
        },
      },
      null,
      2,
    ),
  );

  // A prompt to hand to an assistant that can edit files or run commands, so it
  // does the configuration instead of the person hunting for the right config
  // path. Same masking as everything else here: it carries the token.
  const prompt = $derived(t('mcp.prompt', { url, token }));

  async function copy(what: string, text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      copied = what;
      setTimeout(() => copied === what && (copied = null), 1600);
    } catch {
      copied = null;
    }
  }

  function onKey(e: KeyboardEvent): void {
    if (e.key === 'Escape') onclose();
  }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="scrim" onclick={onclose}>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="box" role="dialog" aria-modal="true" tabindex="-1" onclick={(e) => e.stopPropagation()}>
    <header>
      <h2>{t('mcp.title')}</h2>
      <button class="x" onclick={onclose} aria-label={t('del.cancel')}>×</button>
    </header>

    <p class="lead">{t('mcp.lead')}</p>

    <label class="fld">
      <span class="lbl">{t('mcp.url')}</span>
      <div class="row">
        <code>{url}</code>
        <button onclick={() => copy('url', url)}>
          {copied === 'url' ? t('mcp.copied') : t('mcp.copy')}
        </button>
      </div>
    </label>

    <label class="fld">
      <span class="lbl">{t('mcp.token')}</span>
      <div class="row">
        <code class="tok">{revealed ? token : masked}</code>
        <button onclick={() => (revealed = !revealed)}>
          {revealed ? t('mcp.hide') : t('mcp.reveal')}
        </button>
        <button onclick={() => copy('token', token)}>
          {copied === 'token' ? t('mcp.copied') : t('mcp.copy')}
        </button>
      </div>
    </label>

    <p class="warn">{t('mcp.warn', { who: store.actor?.name ?? '' })}</p>

    <div class="snip">
      <div class="snip-head">
        <span>{t('mcp.claudeCode')}</span>
        <button onclick={() => copy('cli', cli)}>
          {copied === 'cli' ? t('mcp.copied') : t('mcp.copy')}
        </button>
      </div>
      <pre>{revealed ? cli : cli.replace(token, masked)}</pre>
    </div>

    <div class="snip">
      <div class="snip-head">
        <span>{t('mcp.globalConfig')}</span>
        <button onclick={() => copy('json', json)}>
          {copied === 'json' ? t('mcp.copied') : t('mcp.copy')}
        </button>
      </div>
      <pre>{revealed ? json : json.replace(token, masked)}</pre>
    </div>

    <div class="snip">
      <div class="snip-head">
        <span>{t('mcp.promptLabel')}</span>
        <button onclick={() => copy('prompt', prompt)}>
          {copied === 'prompt' ? t('mcp.copied') : t('mcp.copy')}
        </button>
      </div>
      <p class="sub">{t('mcp.promptHint')}</p>
      <pre class="prompt">{revealed ? prompt : prompt.replaceAll(token, masked)}</pre>
    </div>

    <p class="hint">{t('mcp.hint')}</p>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 180;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: oklch(0.12 0.02 265 / 0.55);
    backdrop-filter: blur(4px);
    -webkit-backdrop-filter: blur(4px);
  }
  .box {
    width: min(94vw, 40rem);
    max-height: 86vh;
    overflow-y: auto;
    display: grid;
    gap: 0.7rem;
    padding: 1.25rem 1.35rem 1.4rem;
    border-radius: 18px;
    background: var(--surface-solid);
    border: 1px solid var(--line);
    box-shadow: 0 30px 70px var(--shadow-strong);
    font-family: var(--sans);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }
  h2 {
    margin: 0;
    font-size: 1.02rem;
    font-weight: 800;
    color: var(--text);
  }
  .x {
    border: none;
    background: none;
    color: var(--muted);
    font-size: 1.3rem;
    line-height: 1;
    cursor: pointer;
  }
  p {
    margin: 0;
    font-size: 0.84rem;
    line-height: 1.55;
    color: var(--muted);
  }
  .lead {
    color: var(--text);
  }
  .warn {
    padding: 0.5rem 0.7rem;
    border-radius: 10px;
    border: 1px solid color-mix(in oklab, oklch(0.72 0.19 25) 32%, var(--line));
    background: color-mix(in oklab, oklch(0.72 0.19 25) 8%, transparent);
    font-size: 0.78rem;
  }
  .hint {
    font-size: 0.76rem;
    color: var(--faint);
  }
  .fld {
    display: grid;
    gap: 0.25rem;
  }
  .lbl {
    font-size: 0.72rem;
    font-weight: 700;
    color: var(--muted);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .row code {
    flex: 1;
    min-width: 0;
    padding: 0.42rem 0.6rem;
    border-radius: 9px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    color: var(--text);
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.78rem;
    overflow-x: auto;
    white-space: nowrap;
  }
  .row code.tok {
    letter-spacing: 0.02em;
  }
  button {
    flex: none;
    padding: 0.42rem 0.75rem;
    border-radius: 9px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 6%, transparent);
    color: var(--text);
    font-family: var(--sans);
    font-size: 0.74rem;
    font-weight: 700;
    cursor: pointer;
  }
  button:hover {
    border-color: color-mix(in oklab, var(--wip) 45%, var(--line));
  }
  .snip {
    display: grid;
    gap: 0.3rem;
  }
  .snip-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.72rem;
    font-weight: 700;
    color: var(--muted);
  }
  .sub {
    font-size: 0.74rem;
    color: var(--faint);
  }
  .prompt {
    max-height: 16rem;
    overflow-y: auto;
    white-space: pre-wrap;
    font-family: var(--sans);
    font-size: 0.78rem;
  }
  pre {
    margin: 0;
    padding: 0.65rem 0.75rem;
    border-radius: 11px;
    border: 1px solid var(--line);
    background: color-mix(in oklab, var(--text) 5%, transparent);
    color: var(--text);
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.74rem;
    line-height: 1.55;
    overflow-x: auto;
    white-space: pre;
  }
</style>
