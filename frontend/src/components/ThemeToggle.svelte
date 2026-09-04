<script lang="ts">
  // The theme switch, floating and always reachable.
  //
  // It used to live in the board's header, which meant that every screen without
  // that header — a thread, /theme, sign-in — had no way to change the theme.
  // It is `position: fixed` rather than absolute: it should stay put while the
  // page scrolls, which is the whole point of putting it outside the layout.
  import { theme, type Mode } from '../lib/theme.svelte';

  // Three states, not two: following the system IS a choice, and the default.
  // One control cycling through them beats three controls, at this size.
  const order: Mode[] = ['system', 'light', 'dark'];
  const face: Record<Mode, string> = { system: '🌗', light: '☀️', dark: '🌙' };
  const name: Record<Mode, string> = { system: 'automático', light: 'claro', dark: 'oscuro' };

  const next = $derived(order[(order.indexOf(theme.mode) + 1) % order.length]);
</script>

<button
  class="theme-toggle"
  onclick={() => theme.set(next)}
  title="Tema: {name[theme.mode]} — cambiar a {name[next]}"
  aria-label="tema: {name[theme.mode]}">
  <span aria-hidden="true">{face[theme.mode]}</span>
</button>

<style>
  .theme-toggle {
    position: fixed;
    /* The bottom of the right edge. The minimap owns the middle of it and is
       vertically centred, so the two do not meet. */
    right: 1rem;
    bottom: 1rem;
    z-index: var(--z-chrome);
    width: 42px;
    height: 42px;
    display: grid;
    place-content: center;
    font-size: 1.15rem;
    line-height: 1;
    cursor: pointer;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    transition: transform 0.15s ease, background 0.15s ease;
  }
  .theme-toggle:hover { background: var(--hover); transform: translateY(-1px); }
  .theme-toggle:active { transform: translateY(0); }
</style>
