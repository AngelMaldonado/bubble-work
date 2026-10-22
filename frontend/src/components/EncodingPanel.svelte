<script lang="ts">
  // Qué dicen el tamaño y el color de las burbujas, y qué se resalta: el
  // modelo de lib/encoding, a mano. Abre sobre la barra de abajo.
  import { COLOR_LABELS, DEFAULT_ENCODING, PRESET_LABELS, SIZE_LABELS, sizeApplies, toggleSize, type ColorBy, type Encoding, type Preset, type SizeBy } from '../lib/encoding';

  let { value = $bindable(), level, onclose }: { value: Encoding; level: 'bubbles' | 'threads'; onclose: () => void } = $props();
  const sizes = Object.entries(SIZE_LABELS) as [SizeBy, string][];
  const colors = Object.entries(COLOR_LABELS) as [ColorBy, string][];
  const presets = Object.entries(PRESET_LABELS) as [Preset, string][];
  const set = (patch: Partial<Encoding>) => (value = { ...value, ...patch });
  const toggle = (p: Preset) => set({ presets: value.presets.includes(p) ? value.presets.filter((x) => x !== p) : [...value.presets, p] });
</script>

<div class="enc" role="dialog" aria-label="Tamaño, color y filtros" tabindex="-1"
  onkeydown={(e) => { if (e.key === 'Escape') onclose(); }}>
  <header>
    <b>Qué dicen las burbujas</b>
    <button class="x" aria-label="Cerrar" onclick={onclose}>✕</button>
  </header>
  <div class="row">
    <span class="lbl">Tamaño</span>
    <div>
      <div class="chips" role="group" aria-label="Qué indica el tamaño (se pueden sumar varios)">
        {#each sizes as [k, label] (k)}
          {#if sizeApplies(k, level)}
            <button class:on={value.size.includes(k)} aria-pressed={value.size.includes(k)} onclick={() => set({ size: toggleSize(value.size, k) })}>{label}</button>
          {/if}
        {/each}
      </div>
      <p class="hint">Marca varios para sumarlos; «Igual» los quita.</p>
    </div>
  </div>
  <div class="row">
    <span class="lbl">Color</span>
    <div class="chips" role="group" aria-label="Qué indica el color">
      {#each colors as [k, label] (k)}
        <button class:on={value.color === k} aria-pressed={value.color === k} onclick={() => set({ color: k })}>{label}</button>
      {/each}
    </div>
  </div>
  <div class="row">
    <span class="lbl">Resaltar</span>
    <div class="chips" role="group" aria-label="Filtros">
      {#each presets as [k, label] (k)}
        <button class:on={value.presets.includes(k)} aria-pressed={value.presets.includes(k)} onclick={() => toggle(k)}>{label}</button>
      {/each}
    </div>
  </div>
  <div class="row">
    <span class="lbl">Buscar</span>
    <input class="input" placeholder="Nombre, proyecto o tag…" value={value.text} oninput={(e) => set({ text: e.currentTarget.value })}
      aria-label="Buscar en las burbujas" autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other" />
  </div>
  <footer>
    <div class="chips" role="group" aria-label="Lo que no pasa el filtro">
      <button class:on={value.mode === 'dim'} aria-pressed={value.mode === 'dim'} onclick={() => set({ mode: 'dim' })}>Atenuar lo demás</button>
      <button class:on={value.mode === 'hide'} aria-pressed={value.mode === 'hide'} onclick={() => set({ mode: 'hide' })}>Ocultar lo demás</button>
    </div>
    <button class="reset" onclick={() => (value = { ...DEFAULT_ENCODING })}>Restablecer</button>
  </footer>
</div>

<style>
  .enc { width: min(44rem, calc(100vw - 2rem)); padding: 0.9rem 1rem; border: 1px solid var(--line); border-radius: 14px; background: var(--surface-solid); box-shadow: 0 12px 40px rgb(0 0 0 / 0.3); display: grid; gap: 0.7rem; }
  header, footer { display: flex; align-items: center; justify-content: space-between; gap: 0.6rem; flex-wrap: wrap; }
  .x { width: 28px; height: 28px; border-radius: 999px; }
  .x:hover { background: var(--hover); }
  .row { display: grid; grid-template-columns: 5.5rem minmax(0, 1fr); align-items: start; gap: 0.6rem; }
  .lbl { color: var(--muted); font-size: 0.8rem; padding-top: 0.3rem; }
  .chips { display: flex; flex-wrap: wrap; gap: 0.3rem; }
  .chips button { padding: 0.25rem 0.6rem; border-radius: 999px; border: 1px solid var(--line); font-size: 0.8rem; }
  .chips button:hover { background: var(--hover); }
  .chips button.on { background: color-mix(in oklab, var(--accent) 22%, transparent); border-color: color-mix(in oklab, var(--accent) 60%, var(--line)); font-weight: 650; }
  .input { width: 100%; font-size: 0.85rem; }
  .hint { margin-top: 0.3rem; color: var(--faint); font-size: 0.72rem; }
  .reset { font-size: 0.8rem; color: var(--muted); text-decoration: underline; }
  @media (max-width: 650px) { .row { grid-template-columns: 1fr; gap: 0.3rem; } }
</style>
