<script lang="ts">
  // Un ícono 3D: barro en reposo, color bajo el cursor. El color se enciende
  // con el hover del CONTROL que lo lleva (botón, enlace) y no sólo del dibujo,
  // que es lo que la mano señala; y con `lit`, para lo que está elegido.
  // Sin ícono para lo que se pidió, se dibuja el texto tal cual.
  import { iconFor, iconSrc } from '../lib/icons';

  let {
    of,
    size = '1.25em',
    label = '',
    lit = false,
    tint = false,
    clay = false,
    danger = false,
  }: {
    /** un nombre (`fire`) o un emoji (`🔥`) */
    of: string;
    /** CSS: `1.25em` sigue al texto; `28px` es fijo */
    size?: string;
    /** si el ícono es lo único que dice algo, lo que diría en voz alta */
    label?: string;
    lit?: boolean;
    /** siempre a color, con los colores naturales del objeto */
    tint?: boolean;
    /** A contextual dock is white while empty, even under the pointer. */
    clay?: boolean;
    /** Destructive actions keep the theme's danger color, including hover. */
    danger?: boolean;
  } = $props();

  const filterId = $props.id();
  const name = $derived(iconFor(of));
  // Un archivo que falta no es una imagen rota: se dibuja el emoji de antes.
  let broken = $state(false);
  // Sin variante natural todavía, la de color en su lugar.
  let tintSrc = $state<'tint' | 'color'>('tint');
</script>

{#if name && !broken && danger}
  <span class="icon tinted danger" data-icon={name} style:--icon-size={size}
    role={label ? 'img' : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}>
    <svg class="filter-def" width="0" height="0" aria-hidden="true" focusable="false">
      <defs>
        <filter id={filterId} color-interpolation-filters="sRGB">
          <feFlood flood-color="var(--color-error-500)" result="danger-color" />
          <feBlend in="SourceGraphic" in2="danger-color" mode="multiply" result="shaded" />
          <feComposite in="shaded" in2="SourceAlpha" operator="in" />
        </filter>
      </defs>
    </svg>
    <img src={iconSrc(name, 'clay')} alt="" loading="lazy" decoding="async"
      style:filter={`url(#${filterId})`} onerror={() => (broken = true)} />
  </span>
{:else if name && !broken && tint}
  <span class="icon tinted" data-icon={name} style:--icon-size={size}
    role={label ? 'img' : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}>
    <img src={iconSrc(name, tintSrc)} alt="" loading="lazy" decoding="async"
      onerror={() => (tintSrc === 'tint' ? (tintSrc = 'color') : (broken = true))} />
  </span>
{:else if name && !broken && clay}
  <span class="icon tinted" data-icon={name} style:--icon-size={size} aria-hidden={label ? undefined : 'true'} aria-label={label || undefined}>
    <img src={iconSrc(name, 'clay')} alt="" loading="lazy" decoding="async" onerror={() => (broken = true)} />
  </span>
{:else if name && !broken}
  <span class="icon" class:lit data-icon={name} style:--icon-size={size}
    role={label ? 'img' : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : 'true'}>
    <img class="still" src={iconSrc(name, 'clay')} alt="" loading="lazy" decoding="async" onerror={() => (broken = true)} />
    <img class="glow" src={iconSrc(name, 'color')} alt="" loading="lazy" decoding="async" />
  </span>
{:else}
  <span aria-hidden={label ? undefined : 'true'} aria-label={label || undefined}>{of}</span>
{/if}

<style>
  .filter-def { position: absolute; width: 0; height: 0; pointer-events: none; }
</style>
