<script lang="ts">
  // Conectar a mano: el comando (o la entrada de configuración) de cada harness,
  // con su botón de copiar. Con el token recién generado van completos; sin él,
  // con un hueco que dice qué pegar.
  import { harnessCommands } from '../lib/connect';

  let { url, token }: { url: string; token: string } = $props();

  const items = $derived(harnessCommands(url, token));
  let copied = $state('');
  let error = $state('');

  async function copy(id: string, text: string) {
    try {
      await navigator.clipboard.writeText(text);
      copied = id;
      error = '';
      setTimeout(() => (copied = ''), 1800);
    } catch {
      error = 'El navegador no dejó copiar al portapapeles.';
    }
  }
</script>

<ul class="harness">
  {#each items as h (h.id)}
    <li>
      <div class="hhead">
        <span class="hname">{h.name}</span>
        <button class="btn btn-sm preset-tonal-surface ml-auto" onclick={() => copy(h.id, h.code)} aria-label={`Copiar para ${h.name}`}>
          {copied === h.id ? '✓ copiado' : 'Copiar'}
        </button>
      </div>
      <p class="how">{h.how}</p>
      <pre class="code"><code>{h.code}</code></pre>
    </li>
  {/each}
</ul>
{#if error}<p class="set-err" role="alert">{error}</p>{/if}

<style>
  .harness { display: grid; gap: 1rem; margin: 0; padding: 0; list-style: none; }
  .hhead { display: flex; align-items: center; gap: 0.5rem; }
  .hname { font-size: 0.88rem; font-weight: 700; }
  .how { margin: 0.2rem 0 0.4rem; color: var(--faint); font-size: 0.78rem; line-height: 1.45; }
  .code { margin: 0; padding: 0.55rem 0.75rem; overflow-x: auto; border-radius: 8px; background: var(--hover); font-size: 0.76rem; line-height: 1.5; }
</style>
