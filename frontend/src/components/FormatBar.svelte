<script lang="ts">
 import type {MarkdownEditor} from '../lib/editor';
 let {editor}:{editor:MarkdownEditor|null}=$props();
 const buttons=[{label:'Negrita',key:'B',marker:'**'},{label:'Cursiva',key:'I',marker:'*'},{label:'Subrayado',key:'U',marker:'++'}];
</script>
<div class="format-bar" role="toolbar" aria-label="Formato del texto">
 {#each buttons as b}<button type="button" aria-label={b.label} title={`${b.label} · Ctrl/Cmd+${b.key}`} disabled={!editor} onpointerdown={(e)=>e.preventDefault()} onclick={()=>editor?.wrap(b.marker,'texto')}><span class:italic={b.key==='I'} class:underlined={b.key==='U'}>{b.key}</span></button>{/each}
 <span class="hint">/ para insertar</span>
</div>
<style>
 .format-bar{display:flex;align-items:center;gap:.25rem;padding:.3rem;}
 button{width:2rem;height:2rem;border:1px solid var(--line);border-radius:.5rem;background:var(--surface-solid);font-weight:700;}
 button:hover{background:var(--hover);}.italic{font-style:italic}.underlined{text-decoration:underline}.hint{font-size:.75rem;color:var(--muted);margin-left:.5rem}
</style>
