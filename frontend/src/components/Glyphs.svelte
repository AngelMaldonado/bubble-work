<script lang="ts">
  // Texto con emojis adentro —«🧭 Secuencia», «🔥 Caliente»— dibujado con los
  // íconos 3D donde hay uno para ese emoji, y como texto donde no. Para las
  // etiquetas que llegan armadas como cadena y que sería peor partir a mano.
  import { iconFor } from '../lib/icons';
  import Icon from './Icon.svelte';

  let { text = '', size = '1.25em', tint = false }: { text?: string; size?: string; tint?: boolean } = $props();

  const parts = $derived(
    text.split(/(\p{Extended_Pictographic}\uFE0F?)/u).filter((p) => p !== ''),
  );
</script>

{#each parts as p, i (i)}{#if iconFor(p)}<Icon of={p} {size} {tint} />{:else}{p}{/if}{/each}
