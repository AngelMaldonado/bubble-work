<script lang="ts">
  // Llamas animadas en sitios al azar, para lo que está «Desatado».
  //
  // Son el Lottie «Fire» (LottieFiles Mobile, Lottie Simple License) dibujado
  // una vez en un sprite de 26 fotogramas (public/fx/), sin reproductor Lottie:
  // CSS `steps(26)` a 24 fps. 3 o 4 llamas, cada una en un punto aleatorio de su
  // tramo (no se amontonan), con tamaño y desfase propios; se eligen una vez.
  //
  // El padre tiene que ser `position: relative`. `from`/`to` son el tramo
  // horizontal libre, en %; `place` dice dónde nacen: `top` asoman por encima
  // del borde, `inside` suben desde abajo, `around` brotan a los dos lados de
  // algo (la cabeza del robot), delante y por fuera de él, a alturas al azar.
  let {
    from = 10,
    to = 90,
    place = 'top',
    size = 22,
    opacity = 1,
  }: { from?: number; to?: number; place?: 'top' | 'inside' | 'around'; size?: number; opacity?: number } = $props();

  type Flame = { x: number; y: number; d: number; k: number };
  // svelte-ignore state_referenced_locally
  const flames: Flame[] = (() => {
    const n = Math.random() < 0.5 ? 3 : 4;
    const w = (to - from) / n;
    return Array.from({ length: n }, (_, i) => ({
      // `around`: mitad a la izquierda (−8…20 %) y mitad a la derecha (80…108 %).
      x: place === 'around'
        ? (i % 2 ? 80 : -8) + Math.random() * 28
        : from + w * i + w * (0.15 + Math.random() * 0.7),
      y: place === 'around' ? 5 + Math.random() * 55 : 0,
      d: -Math.random() * 1.083,
      k: 0.7 + Math.random() * 0.45,
    }));
  })();
  // svelte-ignore state_referenced_locally
  const h = Math.round(size * (94 / 64));
</script>

<span class="flames {place}" aria-hidden="true" style="--fw: {size}px; --fh: {h}px; --sheet: {size * 26}px; opacity: {opacity}">
  {#each flames as f, i (i)}
    <i style="left: {f.x.toFixed(1)}%; --y: {f.y.toFixed(0)}%; animation-delay: {f.d.toFixed(2)}s; --k: {f.k.toFixed(2)}"></i>
  {/each}
</span>

<style>
  .flames { position: absolute; inset: 0; z-index: 0; pointer-events: none; }
  i {
    position: absolute;
    width: var(--fw);
    height: var(--fh);
    transform: translateX(-50%) scale(var(--k, 1));
    transform-origin: 50% 100%;
    background: url('/fx/fire-sprite.webp') 0 0 / var(--sheet) var(--fh) no-repeat;
    animation: fury-flame-sprite 1.083s steps(26) infinite;
  }
  .top i { bottom: calc(100% - var(--fh) * 0.38); }
  .inside i { bottom: -6px; }
  .around { z-index: 2; }
  .around i { bottom: var(--y); }
  @keyframes fury-flame-sprite { to { background-position-x: calc(var(--sheet) * -1); } }
  @media (prefers-reduced-motion: reduce) { i { animation: none; } }
</style>
