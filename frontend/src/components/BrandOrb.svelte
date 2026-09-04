<script lang="ts">
  // The mark, ported from v0: a small floating bubble.
  //
  // It is the product in one glyph — a sphere with a sheen, bobbing — so it
  // works with the wordmark beside it and equally well alone, which is what the
  // collapsed rail needs. Same two radial gradients v0 used, over the hot
  // accent, so the logo and the board agree about what the colour means. v0's
  // `--sheen` token has no counterpart here, so the highlight is written out
  // rather than left pointing at a variable nothing defines.
  let { size = 24 }: { size?: number } = $props();
</script>

<span class="brand-orb" style="--s: {size}px" aria-hidden="true"></span>

<style>
  .brand-orb {
    width: var(--s);
    height: var(--s);
    border-radius: 999px;
    flex: none;
    display: block;
    background:
      radial-gradient(circle at 34% 30%, oklch(1 0 0 / 0.75), transparent 42%),
      radial-gradient(circle at 65% 80%, color-mix(in oklab, var(--hot) 60%, transparent), transparent 70%),
      var(--hot);
    box-shadow: inset 0 1px 2px oklch(1 0 0 / 0.45);
    animation: brand-bob 3s ease-in-out infinite;
  }

  @keyframes brand-bob {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-2px); }
  }

  @media (prefers-reduced-motion: reduce) {
    .brand-orb { animation: none; }
  }
</style>
