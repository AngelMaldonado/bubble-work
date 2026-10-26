<script lang="ts">
  // Las burbujas que suben detrás de la puerta (SignIn) y de la bienvenida
  // (Onboarding): el clima de las dos pantallas donde todavía no hay trabajo.
  //
  // The bands, as bubbles that RISE. That is the claim moving: buoyancy is not a
  // colour, it is what the thing does, and a bubble sitting still is a dot.
  //
  // Hand-placed rather than random: a screen that lays itself out differently on
  // every reload cannot be judged, and this one is looked at every morning. The
  // delays are NEGATIVE — each bubble starts mid-flight, so the first frame is
  // the middle of the loop and nobody watches an empty screen fill up.
  //
  // `dur` is the climb, `sway` the sideways drift. They are deliberately not
  // multiples of each other: two periods that divide evenly trace the same line
  // over and over, and the eye catches the repeat.
  const decor = [
    { band: 'hot', size: 190, left: '9%', dur: 34, sway: 11, amp: 26, delay: -4, rest: '58%' },
    { band: 'dormant', size: 110, left: '20%', dur: 27, sway: 8, amp: 18, delay: -19, rest: '18%' },
    { band: 'hot', size: 70, left: '34%', dur: 23, sway: 6.5, amp: 14, delay: -11, rest: '74%' },
    { band: 'closed', size: 140, left: '76%', dur: 38, sway: 13, amp: 30, delay: -25, rest: '26%' },
    { band: 'rip', size: 80, left: '86%', dur: 29, sway: 9.5, amp: 16, delay: -7, rest: '66%' },
    { band: 'dormant', size: 55, left: '64%', dur: 21, sway: 7, amp: 12, delay: -14, rest: '38%' },
    { band: 'hot', size: 120, left: '48%', dur: 31, sway: 10, amp: 22, delay: -28, rest: '10%' },
    { band: 'closed', size: 60, left: '4%', dur: 25, sway: 8.5, amp: 15, delay: -2, rest: '82%' },
    { band: 'rip', size: 45, left: '55%', dur: 19, sway: 5.5, amp: 10, delay: -9, rest: '50%' },
    { band: 'dormant', size: 95, left: '92%', dur: 33, sway: 12, amp: 20, delay: -31, rest: '4%' },
  ];
</script>

<div class="bubbles" aria-hidden="true">
  <!-- Two elements per bubble, not one: the climb and the sway are separate
       animations with separate periods, and a single element can only carry
       one `transform` at a time — the second would overwrite the first. -->
  {#each decor as d, i (i)}
    <span
      class="lift"
      style="--l: {d.left}; --dur: {d.dur}s; --d: {d.delay}s; --rest: {d.rest}">
      <span
        class="blob band-{d.band}"
        style="--s: {d.size}px; --sway: {d.sway}s; --amp: {d.amp}px"></span>
    </span>
  {/each}
</div>

<style>
  /* Behind the card, and behind the page's own grain. They never take a click:
     the form is the only thing here that does anything. */
  .bubbles {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  /* The climb. It starts below the fold and ends above it, so the loop has no
     visible seam: nothing pops in or out where anybody is looking. Linear on
     purpose — a bubble in water rises at a steady rate, and easing here reads as
     the animation restarting rather than as something floating. */
  .lift {
    position: absolute;
    top: 0;
    left: var(--l);
    animation: rise var(--dur) linear infinite;
    animation-delay: var(--d);
    will-change: transform, opacity;
  }
  /* The fade is on the climb, not on the bubble: a bubble that crosses the top
     edge at full strength reads as a slide leaving, and one that appears at the
     bottom edge reads as a slide arriving. This way each one surfaces and
     dissolves, which is what the metaphor claims happens. */
  @keyframes rise {
    0% { transform: translate3d(0, 115vh, 0); opacity: 0; }
    12% { opacity: 1; }
    82% { opacity: 1; }
    100% { transform: translate3d(0, -40vh, 0); opacity: 0; }
  }

  .blob {
    display: block;
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
    animation: sway var(--sway) ease-in-out infinite alternate;
  }
  @keyframes sway {
    from { transform: translate3d(calc(var(--amp) * -1), 0, 0) scale(0.97); }
    to { transform: translate3d(var(--amp), 0, 0) scale(1.04); }
  }

  /* The decoration is the first thing to go: it is motion for its own sake,
     which is exactly what the setting asks not to see. Stopped rather than
     hidden, and stopped SPREAD OUT — freezing the climb at its start would pile
     every bubble below the fold and leave a blank screen. */
  @media (prefers-reduced-motion: reduce) {
    .lift,
    .blob { animation: none; }
    /* `--rest` and not the climb's own start: freezing the animation would pile
       every bubble below the fold and leave a blank screen. */
    .lift { top: var(--rest); }
  }
</style>
