<script lang="ts">
  import Loading from './Loading.svelte';
  import Icon from './Icon.svelte';
  // Un artefacto efímero, donde se contesta. Hoy, un interrogatorio.
  //
  // El agente pregunta desde la terminal; aquí se ve el árbol de decisiones que
  // va saliendo y las preguntas como tarjetas. Cada una trae su recomendación:
  // ✅ la acepta, ✏️ la cambia por lo que tú digas. Al enviar, se le dice al
  // agente «listo» en la terminal y él lee lo contestado.
  //
  // El árbol NO lo dibuja el agente: cada pregunta declara de qué depende, y de
  // eso salen las aristas y los colores — contestada, en la frontera (se puede
  // contestar ya), bloqueada (espera a otra). Derivado, como el calor.
  //
  // Se entera solo: cada entrada nueva llega por el stream.
  import { onDestroy, untrack } from 'svelte';
  import Confirm, { type Doom } from './Confirm.svelte';
  import { ApiError, api, type Artifact, type ArtifactQuestion } from '../lib/api';
  import { live } from '../lib/live.svelte';
  import { ago, when } from '../lib/when';
  import Prose from './Prose.svelte';

  let { id, onback }: { id: string; onback: () => void } = $props();

  let art = $state<Artifact | null>(null);
  let failed = $state('');
  let sending = $state(false);
  let sent = $state(false);
  let deleting = $state(false);
  let doom = $state<Doom>(null);
  let readGeneration = 0;
  let stopped = false;
  const canDelete = $derived(!!art && (art.creator === api.me?.id || api.me?.role === 'lead'));
  /** lo que todavía no se ha enviado */
  let drafts = $state<Record<string, { verdict: 'accepted' | 'override'; text: string }>>({});
  /** preguntas contestadas que se están volviendo a contestar */
  let reopened = $state<Record<string, boolean>>({});

  const isOpen = $derived(art?.status === 'open');
  const questions = $derived(art?.questions ?? []);
  const rounds = $derived([...new Set(questions.map((q) => q.round))].sort((a, b) => a - b));
  const pending = $derived(Object.keys(drafts).length);
  const unanswered = $derived(questions.filter((q) => q.state !== 'answered'));
  const handoff = $derived(art?.entries?.find((e) => e.type === 'handoff'));
  const notes = $derived(art?.entries?.filter((e) => e.type === 'note') ?? []);

  async function load() {
    if (deleting || stopped) return;
    const target=id, owner=api.me?.id, turn=++readGeneration;
    const current = () => !stopped && !deleting && turn===readGeneration && target===id && owner===api.me?.id;
    try {
      const next = await api.artifact(target);
      if (!current()) return;
      art = next;
      failed = '';
    } catch (e) {
      if (current()) {
        if (e instanceof ApiError && e.status===404) { art=null; failed='Este artefacto ya no está disponible.'; }
        else failed = (e as Error).message || 'no se pudo leer';
      }
    }
  }

  function remove() {
    const victim=art, owner=api.me?.id;
    if (!victim || !canDelete || sending || deleting) return;
    doom={
      title: `¿Eliminar «${victim.title}»?`,
      crumbs: ['Artefactos efímeros'],
      body: 'Se eliminará el artefacto y todas sus publicaciones o preguntas y respuestas, sin recuperación. La nota del Inbox y los hilos y documentos duraderos se conservan.' + (pending ? ' También se descartan las respuestas que aún no enviaste.' : ''),
      go: async () => {
        failed=''; deleting=true; ++readGeneration; clearTimeout(soon);
        try {
          await api.deleteArtifact(victim.id);
          if (!stopped && id===victim.id && owner===api.me?.id) onback();
        } catch (e) {
          if (!stopped && id===victim.id && owner===api.me?.id) failed=(e as Error).message || 'No se pudo eliminar';
        } finally {
          if (!stopped && id===victim.id && owner===api.me?.id) deleting=false;
        }
      },
    };
  }

  let soon: ReturnType<typeof setTimeout> | undefined;
  function nudge(data: unknown) {
    const rec = (data as { record?: { artifact?: string; id?: string } } | null)?.record;
    if (rec?.artifact && rec.artifact !== id) return;
    if (!rec?.artifact && rec?.id && rec.id !== id) return;
    clearTimeout(soon);
    soon = setTimeout(load, 200);
  }

  $effect(() => {
    void id;
    void api.me?.id;
    deleting = false;
    sending = false;
    doom = null;
    failed = '';
    art = null;
    drafts = {};
    reopened = {};
    sent = false;
    untrack(() => { void load(); });
  });
  const off1 = live.watch('artifact_entries', nudge);
  const off2 = live.watch('artifacts', nudge);
  // Que un agente esté esperando no viaja por el stream (es una conexión
  // abierta, no una fila), así que se vuelve a preguntar mientras se mira.
  const every = setInterval(load, 20_000);
  onDestroy(() => {
    stopped=true; ++readGeneration;
    clearInterval(every);
    clearTimeout(soon);
    off1();
    off2();
  });

  function accept(q: ArtifactQuestion) {
    drafts[q.id] = { verdict: 'accepted', text: '' };
    sent = false;
  }
  function override(q: ArtifactQuestion) {
    drafts[q.id] = { verdict: 'override', text: drafts[q.id]?.text ?? '' };
    sent = false;
  }
  /** Elegir una opción es contestar con ella; si es la recomendada, aceptarla. */
  function pick(q: ArtifactQuestion, choice: string) {
    const rec = q.recommendation.toLowerCase();
    const tag = choice.match(/^\(([a-z0-9]+)\)/i)?.[1]?.toLowerCase();
    drafts[q.id] = tag && rec.startsWith(`(${tag})`)
      ? { verdict: 'accepted', text: '' }
      : { verdict: 'override', text: choice };
    sent = false;
  }
  function undo(q: ArtifactQuestion) {
    delete drafts[q.id];
    delete reopened[q.id];
  }
  function acceptAll() {
    for (const q of unanswered) if (!drafts[q.id]) drafts[q.id] = { verdict: 'accepted', text: '' };
    sent = false;
  }

  async function send() {
    if (!art || !pending || sending || deleting) return;
    const target=art.id, owner=api.me?.id;
    const current = () => !stopped && !deleting && target===id && owner===api.me?.id;
    const answers = Object.entries(drafts).map(([question, d]) => ({
      question,
      verdict: d.verdict,
      text: d.verdict === 'override' ? d.text.trim() : '',
    }));
    const empty = answers.find((a) => a.verdict === 'override' && !a.text);
    if (empty) {
      failed = `${empty.question}: escribe tu respuesta, o acéptala`;
      return;
    }
    sending = true;
    try {
      const next = await api.answerArtifact(target, answers);
      if (!current()) return;
      art = next;
      drafts = {};
      reopened = {};
      sent = true;
      failed = '';
    } catch (e) {
      if (current()) failed = (e as Error).message || 'no se pudo enviar';
    } finally {
      if (current()) sending = false;
    }
  }

  async function pin() {
    if (!art || deleting) return;
    const target=art.id, owner=api.me?.id;
    const current = () => !stopped && !deleting && target===id && owner===api.me?.id;
    try {
      const next = await api.pinArtifact(target, !art.pinned);
      if (current()) art = next;
    } catch (e) {
      if (current()) failed = (e as Error).message;
    }
  }

  function daysLeft(a: Artifact) {
    if (a.pinned) return 'fijado: no caduca';
    if (!a.expires_at) return '';
    const d = Math.ceil((when(a.expires_at) - Date.now()) / 86_400_000);
    return d <= 0 ? 'caduca hoy' : `caduca en ${d} d`;
  }

  // ---- el árbol ----
  const esc = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  const label = (s: string, max = 70) => {
    const one = s.replace(/[\n\r]+/g, ' ').trim();
    const cut = one.length > max ? one.slice(0, one.lastIndexOf(' ', max) > max * 0.6 ? one.lastIndexOf(' ', max) : max) + '…' : one;
    return cut.replace(/"/g, '#quot;');
  };
  const node = (qid: string) => 'n_' + qid.replace(/[^A-Za-z0-9_]/g, '_');

  const tree = $derived.by(() => {
    if (!art || !questions.length) return '';
    const lines = ['flowchart TD', `  root(["${label(art.title, 110)}"])`];
    const known = new Set(questions.map((q) => q.id));
    for (const q of questions) {
      const glyph = q.state === 'answered' ? '✔ ' : q.state === 'frontier' ? '❓ ' : '';
      lines.push(`  ${node(q.id)}["${glyph}${label(q.id)} · ${label(q.title)}"]`);
      const deps = (q.depends_on ?? []).filter((d) => known.has(d));
      if (!deps.length) lines.push(`  root --> ${node(q.id)}`);
      for (const d of deps) lines.push(`  ${node(d)} --> ${node(q.id)}`);
      lines.push(`  class ${node(q.id)} ${q.state}`);
    }
    lines.push(
      '  classDef answered fill:#14532d,stroke:#22c55e,color:#ecfdf5',
      '  classDef frontier fill:#9a3412,stroke:#f97316,color:#fff7ed,stroke-width:2px',
      '  classDef blocked fill:#1e293b,stroke:#475569,color:#94a3b8',
    );
    return `<pre><code class="language-mermaid">${esc(lines.join('\n'))}</code></pre>`;
  });

  /** Clic en un nodo del árbol lleva a su tarjeta. */
  function jump(e: MouseEvent) {
    const g = (e.target as Element).closest('g.node');
    const m = g?.id.match(/n_([A-Za-z0-9_]+)/);
    if (!m) return;
    const q = questions.find((x) => node(x.id) === 'n_' + m[1]);
    if (q) document.getElementById('q-' + q.id)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
</script>

<div class="art">
  <header class="bar">
    <button class="btn btn-sm preset-tonal-surface" onclick={onback}>← volver</button>
    {#if art}
      <div class="head">
        <span class="kind"><Icon of={art.kind === 'markdown' ? 'document' : 'fire'} tint /> {art.kind === 'markdown' ? 'documento .md · sólo lectura' : 'interrogatorio'}</span>
        <h1>{art.title}</h1>
      </div>
      <div class="facts">
        {#if art.watched}
          <span class="chip watched"><Icon of="eyes" tint /> tu agente espera tus respuestas</span>
        {:else if art.live}
          <span class="chip live"><span class="dot"></span> agente activo</span>
        {/if}
        {#if !isOpen}<span class="chip"><Icon of="check" tint /> traspasado</span>{/if}
        {#if art.kind === 'grill'}<span class="chip">{art.answered}/{art.total} contestadas</span>{:else if art.document}<span class="chip">revisión {art.document.seq}</span>{/if}
        <button class="chip pin" class:on={art.pinned} onclick={pin} disabled={deleting}
          title={art.pinned ? 'Dejar que caduque' : 'Fijar: que no caduque'}>
          <Icon of="pin" tint /> {daysLeft(art)}
        </button>
        {#if canDelete}<button class="chip delete-artifact" onclick={remove} disabled={sending || deleting}>
          {#if deleting}<Loading compact label="Eliminando artefacto" />{:else}<Icon of="trash" danger /> Eliminar artefacto{/if}
        </button>{/if}
      </div>
    {/if}
  </header>

  {#if failed}<p class="oops" role="alert">{failed}</p>{/if}

  {#if !art && !failed}
    <div class="artifact-loading"><Loading label="Cargando artefacto" /></div>
  {:else if art?.kind === 'markdown'}
    <section class="published">
      {#if art.document}
        <h2>{art.document.filename}</h2>
        <Prose html={art.document.html ?? ''} scrollKey={`artifact.${art.id}.document`} />
      {:else}<p class="muted">El agente aún no ha publicado este documento.</p>{/if}
      {#if !isOpen}<p class="muted">Traspasado. El trabajo duradero vive en sus destinos; este documento sigue siendo efímero.</p>{/if}
    </section>
  {:else if art}
    <div class="body">
      <section class="tree">
        <h2>Árbol de decisiones</h2>
        {#if tree}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <!-- Remontado en cada cambio del árbol: mermaid cambia el bloque por
               un SVG, y {@html} ya no encuentra el nodo que tiene que quitar —
               sin el key, cada ronda nueva dibujaba otro árbol debajo. -->
          <div class="diagram" onclick={jump}>
            {#key tree}<Prose html={tree} compact />{/key}
          </div>
          <p class="legend">
            <span class="sw answered"></span> contestada
            <span class="sw frontier"></span> se puede contestar ya
            <span class="sw blocked"></span> espera a otra
          </p>
        {:else}
          <p class="muted">El agente todavía no ha preguntado nada. Aparece aquí en cuanto lo haga.</p>
        {/if}
        {#if notes.length}
          <h2>Notas del agente</h2>
          {#each notes as n (n.seq)}
            <div class="note"><Prose html={n.html ?? ''} compact /></div>
          {/each}
        {/if}
      </section>

      <section class="questions">
        {#if handoff}
          <div class="handed">
            <Icon of="check" tint /> Traspasado {ago(handoff.at ?? '')}. El trabajo vive ahora en
            {((handoff.data.targets as { kind: string }[]) ?? []).length} destino(s) — este registro es de
            sólo lectura y caduca salvo que lo fijes.
          </div>
        {/if}

        {#each rounds as r (r)}
          <h2>Ronda {r}</h2>
          {#each questions.filter((q) => q.round === r) as q (q.id)}
            {@const d = drafts[q.id]}
            {@const editing = isOpen && (q.state !== 'answered' || reopened[q.id])}
            <article id="q-{q.id}" class="card q {q.state}" class:drafted={!!d}>
              <header>
                <span class="qid">{q.id}</span>
                <h3>{q.title}</h3>
                {#if q.state === 'blocked'}
                  <span class="wait">espera a {(q.depends_on ?? []).join(', ')}</span>
                {/if}
              </header>
              <Prose html={q.body_html ?? ''} compact />
              {#if q.choices?.length}
                <div class="choices">
                  {#each q.choices as c (c)}
                    <button class="pick choice" disabled={!editing}
                      class:on={d?.verdict === 'override' && d.text === c}
                      onclick={() => pick(q, c)}>{c}</button>
                  {/each}
                </div>
              {/if}
              <div class="rec">
                <span class="arrow"><Icon of="arrow" size="1.4em" tint /></span>
                <Prose html={q.recommendation_html ?? ''} compact />
              </div>

              {#if q.answer && !reopened[q.id]}
                <p class="given">
                  {#if q.answer.verdict === 'accepted'}<Icon of="check" tint /> Aceptada{:else}<Icon of="pencil" tint /> Tu respuesta{/if}
                  <span class="via">· {q.answer.via === 'harness' ? 'en la terminal' : 'aquí'}</span>
                  {#if isOpen}
                    <button class="link" onclick={() => (reopened[q.id] = true)}>cambiar</button>
                  {/if}
                </p>
                {#if q.answer.verdict === 'override' && q.answer.html}
                  <div class="given-text"><Prose html={q.answer.html} compact /></div>
                {/if}
              {/if}

              {#if editing}
                <div class="verbs">
                  <button class="pick" class:on={d?.verdict === 'accepted'} onclick={() => accept(q)}>
                    <Icon of="check" /> Aceptar
                  </button>
                  <button class="pick" class:on={d?.verdict === 'override'} onclick={() => override(q)}>
                    <Icon of="pencil" /> Otra respuesta
                  </button>
                  {#if d || reopened[q.id]}
                    <button class="link" onclick={() => undo(q)}>deshacer</button>
                  {/if}
                </div>
                {#if d?.verdict === 'override'}
                  <textarea class="textarea" rows="3" placeholder="Tu respuesta, y por qué"
                    bind:value={drafts[q.id].text}></textarea>
                {/if}
              {/if}
            </article>
          {/each}
        {/each}
      </section>
    </div>

    {#if isOpen && (unanswered.length || pending || sent)}
      <footer class="send">
        {#if sent && !pending}
          {#if art.watched}
            <span>Enviado. <Icon of="eyes" tint /> Tu agente ya se enteró: espera la siguiente ronda aquí.</span>
          {:else}
            <span>Enviado. Dile a tu agente <b>«listo»</b> en la terminal para la siguiente ronda.</span>
          {/if}
        {:else}
          <span class="muted">{pending} por enviar · {unanswered.length} sin contestar</span>
          {#if unanswered.length}
            <button class="btn btn-sm preset-tonal-surface" onclick={acceptAll}>Aceptar todas</button>
          {/if}
          <button class="btn btn-sm preset-filled-primary-500" disabled={!pending || sending} onclick={send}>
            {#if sending}<Loading compact label="Enviando respuestas" />{:else}Enviar {pending || ''} respuesta{pending === 1 ? '' : 's'}{/if}
          </button>
        {/if}
      </footer>
    {/if}
  {/if}
</div>

<Confirm bind:ask={doom} />

<style>
  .chip.delete-artifact { color: var(--color-error-500); }
  .chip.delete-artifact:hover { background: color-mix(in oklab, var(--color-error-500) 10%, transparent); }
  .published { max-width: 1200px; margin: 2rem auto; padding: 0 1.5rem; }
  .published h2 { color: var(--faint); font-size: 0.9rem; margin-bottom: 1.5rem; }
  .artifact-loading { height: calc(100dvh - 10rem); min-height: 10rem; }
  .art { min-height: 100vh; padding-bottom: 6rem; }
  .bar {
    position: sticky;
    top: 0;
    z-index: var(--z-chrome);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.8rem;
    padding: 0.7rem 1.2rem;
    border-bottom: 1px solid var(--line);
    background: var(--surface-solid);
  }
  .head { display: flex; flex-direction: column; min-width: 0; flex: 1; }
  .kind { font-size: 0.72rem; color: var(--muted); }
  h1 { font-size: 1.15rem; font-weight: 750; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .facts { display: flex; flex-wrap: wrap; gap: 0.4rem; }
  .chip {
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    font-size: 0.75rem;
    background: transparent;
    color: inherit;
  }
  .chip.live { border-color: tomato; display: inline-flex; align-items: center; gap: 0.4rem; }
  .chip .dot { width: 8px; height: 8px; border-radius: 999px; background: tomato; }
  .chip.watched { border-color: #22c55e; color: #4ade80; }
  .pin { cursor: pointer; }
  .pin.on { border-color: var(--accent); color: var(--accent); }
  .oops { margin: 0.8rem 1.2rem; color: var(--p1, tomato); font-size: 0.85rem; }

  /* Una columna, como un documento: el mismo ancho que la prosa de un hilo
     (Prose: 940px). Arriba el árbol y las notas; debajo, las rondas en orden. */
  .body {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    padding: 1.2rem;
    max-width: 940px;
    margin: 0 auto;
  }
  h2 { margin: 0.8rem 0; font-size: 0.9rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); }
  .diagram { cursor: pointer; }
  .diagram :global(.prose) { max-width: none; }
  .legend { display: flex; align-items: center; gap: 0.4rem; flex-wrap: wrap; font-size: 0.88rem; color: var(--muted); }
  .sw { width: 10px; height: 10px; border-radius: 3px; display: inline-block; margin-left: 0.5rem; }
  .sw.answered { background: #22c55e; }
  .sw.frontier { background: #f97316; }
  .sw.blocked { background: #475569; }
  .note { padding: 0.4rem 0.7rem; border-left: 2px solid var(--line); margin: 0.3rem 0; }
  .note :global(.prose) { font-size: 17px; line-height: 1.6; }

  .q { padding: 1.3rem 1.5rem; margin-bottom: 1.1rem; border: 1px solid var(--line); border-radius: 14px; scroll-margin-top: 5rem; }
  .q.frontier { border-color: #f97316; }
  .q.answered { opacity: 0.85; }
  .q.blocked { opacity: 0.6; }
  .q.drafted { box-shadow: 0 0 0 2px color-mix(in oklab, var(--accent) 45%, transparent); }
  .q header { display: flex; align-items: baseline; gap: 0.5rem; flex-wrap: wrap; margin-bottom: 0.3rem; }
  .qid { font-family: var(--mono, ui-monospace, monospace); font-size: 0.85rem; color: var(--muted); }
  h3 { font-size: 1.3rem; font-weight: 750; line-height: 1.25; }
  .wait { font-size: 0.85rem; color: var(--muted); }
  /* El tamaño de la prosa de un hilo (Prose: 18px): una pregunta se lee como un documento. */
  .q :global(.prose) { font-size: 18px; line-height: 1.65; }
  .rec { display: flex; gap: 0.5rem; align-items: baseline; margin-top: 0.9rem; padding: 0.75rem 1rem;
    border-radius: 10px; background: color-mix(in oklab, var(--accent) 9%, transparent); }
  .rec :global(.prose) { font-weight: 600; }
  .rec :global(.prose > :last-child), .note :global(.prose > :last-child),
  .given-text :global(.prose > :last-child) { margin-bottom: 0; }
  .choices { display: flex; flex-wrap: wrap; gap: 0.5rem; margin-top: 0.8rem; }
  .verbs { display: flex; align-items: center; gap: 0.6rem; margin-top: 0.9rem; flex-wrap: wrap; }

  /* Un botón que parece botón: fondo sólido, borde, sombra, y se levanta bajo
     el cursor. Elegido, se rellena con el acento. Las opciones y los verbos son
     el mismo gesto —contestar— y por eso llevan la misma forma. */
  .pick {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.55rem 1.05rem;
    border: 1px solid color-mix(in oklab, var(--text) 18%, var(--line));
    border-radius: 12px;
    background: color-mix(in oklab, var(--text) 7%, var(--surface-solid));
    color: var(--text);
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.2;
    cursor: pointer;
    box-shadow: 0 1px 0 rgb(255 255 255 / 0.06) inset, 0 3px 8px rgb(0 0 0 / 0.28);
    transition: transform 0.12s ease, background 0.12s ease, border-color 0.12s ease, box-shadow 0.12s ease;
  }
  .pick:hover:not(:disabled) {
    transform: translateY(-1px);
    border-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 14%, var(--surface-solid));
    box-shadow: 0 1px 0 rgb(255 255 255 / 0.08) inset, 0 6px 14px rgb(0 0 0 / 0.32);
  }
  .pick:active:not(:disabled) { transform: translateY(0); box-shadow: 0 1px 3px rgb(0 0 0 / 0.3); }
  .pick:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .pick.on {
    border-color: var(--accent);
    background: var(--accent);
    color: white;
  }
  .pick:disabled { cursor: default; opacity: 0.6; box-shadow: none; }
  @media (prefers-reduced-motion: reduce) { .pick { transition: none; } .pick:hover { transform: none; } }

  .given-text { margin-top: 0.3rem; padding-left: 0.7rem; border-left: 2px solid var(--accent); }
  .given { margin-top: 0.7rem; font-size: 1rem; }
  .via { color: var(--muted); font-size: 0.85rem; }
  .link { background: none; border: none; color: var(--accent); cursor: pointer; font-size: 0.92rem; text-decoration: underline; }
  textarea { width: 100%; margin-top: 0.7rem; font-size: 1rem; padding: 0.6rem 0.8rem; }
  .handed { padding: 0.7rem 0.9rem; margin-bottom: 1rem; border: 1px solid #22c55e; border-radius: 12px; font-size: 0.85rem; }

  .send {
    position: fixed;
    left: 50%;
    bottom: 1rem;
    transform: translateX(-50%);
    z-index: var(--z-chrome);
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.5rem 0.6rem 0.5rem 1rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--surface-solid);
    box-shadow: 0 10px 30px rgb(0 0 0 / 0.25);
    font-size: 0.85rem;
    max-width: calc(100vw - 8rem);
  }
</style>
