<script lang="ts">
  // The door. It is the first thing anybody sees, and until now it said the
  // product's name in a box with two fields — true, and indistinguishable from
  // every other login.
  //
  // What it says instead: the claim, in the app's own face, over bubbles doing
  // the one thing the product is about. The bubbles are decoration and are
  // marked as such (`aria-hidden`, and gone under reduced motion): a screen
  // reader gets a form, not a lava lamp.
  import { api } from '../lib/api';
  import BrandOrb from './BrandOrb.svelte';

  let { onDone }: { onDone: () => void } = $props();
  let identity = $state('');
  // Whether anybody can sign in at all. A superuser is not a person: it operates
  // the box and has no row in `users`.
  let empty = $state(false);
  (async () => {
    try {
      const res = await fetch('/api/collections/users/records?perPage=1');
      if (res.ok) empty = (await res.json()).totalItems === 0;
    } catch {
      /* the server will say so when the form is submitted */
    }
  })();
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  // The four bands, as four bubbles, in the order the board stacks them. Sizes
  // and offsets are hand-placed rather than random: a layout that changes on
  // every reload cannot be judged, and this one is looked at every morning.
  const decor = [
    { band: 'hot', size: 190, top: '14%', left: '10%', delay: '0s' },
    { band: 'dormant', size: 110, top: '64%', left: '18%', delay: '1.1s' },
    { band: 'rip', size: 80, top: '24%', left: '82%', delay: '2.3s' },
    { band: 'closed', size: 140, top: '70%', left: '78%', delay: '0.6s' },
  ];

  async function submit(e: Event) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.signIn(identity, password);
      onDone();
    } catch (err) {
      // PocketBase answers "Failed to authenticate." whether the person does not
      // exist or the password is wrong. When NOBODY exists, that reads as a bug in
      // your typing rather than an empty database — so say which it is.
      const msg = (err as Error).message;
      error = empty
        ? 'Todavía no hay ninguna persona en este servidor. Créala con `just person <correo> <contraseña> lead` — un superuser opera la caja, pero no trabaja aquí.'
        : msg;
    } finally {
      busy = false;
    }
  }
</script>

<div class="door">
  <div class="bubbles" aria-hidden="true">
    {#each decor as d (d.band)}
      <span
        class="blob band-{d.band}"
        style="--s: {d.size}px; --t: {d.top}; --l: {d.left}; --d: {d.delay}"></span>
    {/each}
  </div>

  <form class="card glass sheet" onsubmit={submit}>
    <div class="wordmark">
      <BrandOrb size={30} />
      <h1 class="display text-2xl">bubble.work</h1>
    </div>
    <!-- The claim, not a tagline: it is the sentence the whole model follows
         from, and somebody who reads it once understands the board. -->
    <p class="claim">La atención se comporta como la flotabilidad.<br />
      <span class="faint">Lo que produce sube; lo que se calla, se hunde.</span></p>

    <label class="field">
      <span class="muted">Correo</span>
      <input
        class="input mt-1"
        type="email" bind:value={identity} autocomplete="username" required />
    </label>
    <label class="field">
      <span class="muted">Contraseña</span>
      <input
        class="input mt-1"
        type="password" bind:value={password} autocomplete="current-password" required />
    </label>

    {#if error}
      <p class="mb-3 text-sm text-error-500">{error}</p>
    {/if}

    <!-- The app's accent, not Skeleton's `primary`: primary is a cyan nothing
         else on any screen uses, and the one button on the first screen should
         be the colour the product actually is. -->
    <button class="enter" disabled={busy}>{busy ? '…' : 'Entrar'}</button>
  </form>
</div>

<style>
  .door {
    position: relative;
    display: grid;
    place-items: center;
    min-height: 100vh;
    padding: 1.5rem;
    overflow: hidden;
  }

  /* Behind the card, and behind the page's own grain. They never take a click:
     the form is the only thing here that does anything. */
  .bubbles {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .blob {
    position: absolute;
    top: var(--t);
    left: var(--l);
    width: var(--s);
    height: var(--s);
    border-radius: 999px;
    /* The same two gradients the mark uses, over the band's accent, so a bubble
       here and a bubble on the board are recognisably the same object. */
    background:
      radial-gradient(circle at 34% 30%, oklch(1 0 0 / 0.5), transparent 45%),
      radial-gradient(circle at 65% 80%, color-mix(in oklab, var(--accent) 55%, transparent), transparent 70%),
      color-mix(in oklab, var(--accent) 42%, transparent);
    /* Blurred and faint: they are the WEATHER of the screen, and a decoration
       that competes with the one form on it is a decoration that won. */
    filter: blur(14px);
    opacity: 0.34;
    animation: float 9s ease-in-out infinite;
    animation-delay: var(--d);
  }
  @keyframes float {
    0%, 100% { transform: translateY(0) scale(1); }
    50% { transform: translateY(-16px) scale(1.02); }
  }

  .sheet {
    position: relative;
    width: 100%;
    max-width: 24rem;
    padding: 2rem 1.75rem;
  }
  /* `wordmark`, not `mark`: Skeleton owns `.mark` (its highlight utility —
     tertiary background, contrast text), so the logo row came out as an orange
     bar with white lettering. Third time this bites: `.card` and `.label` were
     the first two. */
  .wordmark { display: flex; align-items: center; gap: 0.6rem; }
  .claim {
    margin: 0.75rem 0 1.5rem;
    font-size: 0.9rem;
    line-height: 1.45;
    color: var(--muted);
  }
  .field { display: block; margin-bottom: 0.9rem; font-size: 0.875rem; }
  .field:last-of-type { margin-bottom: 1.25rem; }
  .enter {
    width: 100%;
    padding: 0.55rem 1rem;
    border-radius: 10px;
    border: 1px solid color-mix(in oklab, var(--hot) 70%, black);
    background: var(--hot);
    color: oklch(0.99 0 0);
    font-weight: 650;
    transition: filter 0.15s ease, transform 0.1s ease;
  }
  .enter:hover:not(:disabled) { filter: brightness(1.06); }
  .enter:active:not(:disabled) { transform: translateY(1px); }
  .enter:disabled { opacity: 0.6; }

  /* The decoration is the first thing to go: it is motion for its own sake,
     which is exactly what the setting asks not to see. */
  @media (prefers-reduced-motion: reduce) {
    .blob { animation: none; }
  }
</style>
