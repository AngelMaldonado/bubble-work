<script lang="ts">
  import TeamPick from './TeamPick.svelte';
  import FilterPick from './FilterPick.svelte';
  // Canales: por dónde entra al inbox lo que no se escribe en la app.
  //
  // Cada canal es un plugin del servidor; esta pantalla enseña los que el
  // binario trae. Cada EQUIPO conecta el suyo —su propia cuenta, su propio
  // grupo— y lo que entra va al inbox de ese equipo: las ideas se quedan en su
  // contexto. WhatsApp escucha UN grupo: se vincula una cuenta (mejor un
  // número dedicado), se elige el grupo, y cada mensaje que llega ahí entra al
  // inbox y recibe 📥.
  import { Switch } from '@skeletonlabs/skeleton-svelte';
  import { api, type ChannelStatus } from '../lib/api';
  import Confirm, { type Doom } from './Confirm.svelte';
  import { untrack } from 'svelte';
  import { teams } from '../lib/teams.svelte';

  /** Los equipos que lideras: de esos conectas el canal. */
  const led = $derived(teams.list.filter((t) => t.role === 'lead'));
  let team = $state(untrack(() => (teams.leads(teams.current) ? teams.current : (teams.list.find((t) => t.role === 'lead')?.id ?? ''))));

  let rows = $state<ChannelStatus[]>([]);
  let error = $state('');
  let busy = $state(false);
  let doom = $state<Doom>(null);

  async function load() {
    try {
      rows = await api.channels(team);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    void team;
    untrack(() => { rows = []; groups = []; phoneCode = ''; load(); });
  });

  const wa = $derived(rows.find((r) => r.kind === 'whatsapp'));
  const extra = (k: string) => String(wa?.extra?.[k] ?? '');
  // Un primitivo, a propósito: el efecto de abajo recarga `rows` cada dos
  // segundos, y depender del objeto lo volvería a disparar en cada recarga.
  const waState = $derived(wa?.state ?? 'disabled');

  // Mientras se vincula o conecta, el estado cambia solo (el QR rota cada ~20 s):
  // se pregunta cada dos segundos, y sólo entonces.
  let qrUrl = $state('');
  $effect(() => {
    const live = waState === 'pairing' || waState === 'connecting';
    if (!live) {
      qrUrl = '';
      return;
    }
    const tick = async () => {
      await load();
      if (rows.find((r) => r.kind === 'whatsapp')?.state === 'pairing' && !phoneCode) {
        const next = await api.channelQR('whatsapp', team);
        if (qrUrl) URL.revokeObjectURL(qrUrl);
        qrUrl = next;
      }
    };
    tick();
    const t = setInterval(tick, 2000);
    return () => clearInterval(t);
  });

  async function run<T>(p: Promise<T>): Promise<T | undefined> {
    busy = true;
    error = '';
    try {
      return await p;
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
      await load();
    }
  }

  // ---- vincular ----
  let phone = $state('');
  let phoneCode = $state('');
  async function pairPhone() {
    const out = await run(api.channelAction<{ code: string }>('whatsapp', 'pair-phone', { phone }, team));
    if (out?.code) phoneCode = out.code;
  }

  // ---- grupo ----
  let groups = $state<{ id: string; name: string }[]>([]);
  // El dropdown: los grupos de la cuenta; si todavía no llegan, al menos el que
  // se escucha, para que no aparezca vacío.
  const groupItems = $derived.by(() => {
    const out = groups.map((g) => ({ value: g.id, label: g.name || g.id }));
    const id = extra('group_id');
    if (id && !out.some((g) => g.value === id)) out.unshift({ value: id, label: extra('group_name') || id });
    return out;
  });
  async function loadGroups() {
    groups = (await run(api.channelAction<{ id: string; name: string }[]>('whatsapp', 'groups', {}, team))) ?? [];
  }
  $effect(() => {
    if (waState === 'connected' && untrack(() => !groups.length)) untrack(loadGroups);
  });
  function chooseGroup(body: { id?: string; name?: string }) {
    run(api.channelAction('whatsapp', 'group', body, team));
  }

  const stateText: Record<string, string> = {
    disabled: 'apagado',
    unlinked: 'sin cuenta vinculada',
    pairing: 'vinculando…',
    connecting: 'conectando…',
    connected: 'conectado',
    error: 'error',
  };
</script>

<section class="set-section">
  <p class="set-lead">
    Por dónde entra al inbox lo que no se escribe en la app. Cada equipo tiene su propio canal, con
    su propia cuenta y su propio grupo: lo que llega va al inbox de ESE equipo, a nombre de quien
    enciende el canal, y dice quién lo mandó.
  </p>
  {#if led.length > 1}
    <div class="set-row team-row">
      <span>Canal del equipo</span>
      <TeamPick teams={led} value={team} label="Canal del equipo" onchange={(id) => (team = id)} />
    </div>
  {:else if led.length === 1}
    <p class="faint text-sm">Canal del equipo <b>{led[0].name}</b>.</p>
  {/if}

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  {#if wa}
    {@const account = extra('account')}
    <div class="set-card wa">
      <div class="set-row head">
        <span class="logo" aria-hidden="true"></span>
        <h3>WhatsApp</h3>
        <span class="state {wa.state}">{stateText[wa.state] ?? wa.state}</span>
        <Switch
          class="ml-auto"
          checked={wa.enabled}
          disabled={busy}
          onCheckedChange={(e: { checked: boolean }) => run(api.setChannel('whatsapp', e.checked, team))}>
          <Switch.Control><Switch.Thumb /></Switch.Control>
          <Switch.HiddenInput />
        </Switch>
      </div>

      <p class="help">
        Escucha <b>un grupo</b>: cada mensaje que llega ahí entra al inbox y recibe la reacción 📥.
        Usa un número dedicado: la conexión no es la API oficial de WhatsApp, y una cuenta que manda
        mensajes puede ser bloqueada. Este canal sólo escucha y reacciona.
      </p>

      {#if wa.detail}<p class="detail">{wa.detail}</p>{/if}

      <!-- El número: vinculado, se ve (y no se edita: para cambiarlo se
           desvincula); si no, se dice que falta, sin rodeos. -->
      <div class="block">
        <span class="label">Número vinculado</span>
        {#if account}
          <div class="set-row">
            <input class="input grow" disabled value={extra('push_name') ? `${account} · ${extra('push_name')}` : account} aria-label="Número vinculado" />
            {#if wa.enabled}
              <button
                class="btn btn-sm preset-tonal-surface"
                disabled={busy}
                onclick={() =>
                  (doom = {
                    title: '¿Desvincular WhatsApp?',
                    body: 'Se cierra la sesión de este dispositivo en la cuenta. Deja de entrar lo del grupo hasta que vuelvas a vincular.',
                    verb: 'Desvincular',
                    go: () => {
                      phoneCode = '';
                      groups = [];
                      run(api.channelAction('whatsapp', 'unlink', {}, team));
                    },
                  })}>
                Desvincular
              </button>
            {/if}
          </div>
        {:else}
          <p class="need" role="status">
            <b>Falta vincular un número.</b>
            {wa.enabled ? 'Sin cuenta vinculada no entra nada: vincúlala con el QR o con un código.' : 'Enciende el canal para vincular la cuenta.'}
          </p>
          {#if wa.enabled && (wa.state === 'unlinked' || wa.state === 'pairing')}
            {#if wa.state === 'pairing' && qrUrl && !phoneCode}
              <img class="qr" src={qrUrl} alt="Código QR para vincular WhatsApp" />
              <p class="help">WhatsApp → Ajustes → Dispositivos vinculados → Vincular un dispositivo.</p>
            {:else if phoneCode}
              <p class="code">{phoneCode}</p>
              <p class="help">
                WhatsApp → Dispositivos vinculados → Vincular un dispositivo → «Vincular con número de
                teléfono», y escribe este código.
              </p>
            {:else}
              <div class="set-row">
                <button class="btn btn-sm preset-filled-primary-500" disabled={busy} onclick={() => run(api.channelAction('whatsapp', 'pair', {}, team))}>
                  Vincular con QR
                </button>
                <span class="faint text-xs">o</span>
                <input class="input phone" placeholder="52 55 1234 5678" bind:value={phone} aria-label="Número para pedir el código" />
                <button class="btn btn-sm preset-tonal-surface" disabled={busy || !phone.trim()} onclick={pairPhone}>
                  Pedir código
                </button>
              </div>
            {/if}
          {/if}
        {/if}
      </div>

      {#if account}
        <div class="block">
          <span class="label">El grupo que se escucha</span>
          {#if wa.enabled && wa.state === 'connected'}
            <div class="set-row">
              <span class="grow">
                <FilterPick items={groupItems} value={extra('group_id')} any="Elige el grupo…" label="Grupo de WhatsApp"
                  onchange={(id) => id && id !== extra('group_id') && chooseGroup({ id })} />
              </span>
              <button class="btn btn-sm preset-tonal-surface" disabled={busy} onclick={loadGroups}>Actualizar la lista</button>
            </div>
            {#if !extra('group_id')}<p class="need">Todavía no hay grupo: no entra nada.</p>{/if}
          {:else if extra('group_id')}
            <p class="help">«{extra('group_name')}» — se escucha cuando el canal está encendido y conectado.</p>
          {:else}
            <p class="help">Se elige cuando el canal está encendido y conectado.</p>
          {/if}
        </div>
      {/if}
    </div>
  {:else if !error}
    <p class="faint text-sm">Este servidor no trae canales.</p>
  {/if}
</section>

<Confirm bind:ask={doom} />

<style>
  .head h3 { margin: 0; }
  /* El logo de WhatsApp (Simple Icons, una sola figura) en su verde. */
  .logo { width: 22px; height: 22px; flex: none; background: #25d366; -webkit-mask: url('/logos/whatsapp.svg') center / contain no-repeat; mask: url('/logos/whatsapp.svg') center / contain no-repeat; }
  .state { padding: 0.15rem 0.6rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.78rem; font-weight: 700; }
  .state.connected { background: color-mix(in oklab, var(--color-success-500) 22%, transparent); color: var(--text); }
  .state.error { background: color-mix(in oklab, var(--color-error-500) 18%, transparent); color: var(--text); }
  .help { margin: 0.4rem 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .detail { margin: 0.4rem 0; color: var(--muted); font-size: 0.82rem; }
  .block { display: flex; flex-direction: column; gap: 0.5rem; margin-top: 1.1rem; }
  .label { font-size: 0.85rem; font-weight: 700; }
  .block p { margin: 0; font-size: 0.85rem; }
  /* Lo que falta para que el canal sirva, dicho de frente. */
  .need { padding: 0.5rem 0.75rem; border-radius: 8px; background: color-mix(in oklab, var(--color-warning-500) 16%, transparent); color: var(--text); }
  .qr { width: 240px; height: 240px; border-radius: 10px; background: white; padding: 8px; image-rendering: pixelated; }
  .code { margin: 0; font-family: ui-monospace, monospace; font-size: 1.6rem; font-weight: 700; letter-spacing: 0.15em; }
  .phone { width: 12rem; }
  .grow { flex: 1; min-width: 12rem; }
  .team-row { gap: 0.6rem; align-items: center; margin-bottom: 0.75rem; }
</style>
