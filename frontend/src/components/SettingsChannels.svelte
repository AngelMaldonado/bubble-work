<script lang="ts">
  // Canales: por dónde entra al inbox lo que no se escribe en la app.
  //
  // Cada canal es un plugin del servidor; esta pantalla enseña los que el
  // binario trae. WhatsApp escucha UN grupo: se vincula una cuenta (mejor un
  // número dedicado), se elige el grupo, y cada mensaje que llega ahí entra al
  // inbox y recibe 📥.
  import { Switch } from '@skeletonlabs/skeleton-svelte';
  import { api, type ChannelStatus } from '../lib/api';
  import Confirm, { type Doom } from './Confirm.svelte';
  import { untrack } from 'svelte';

  let rows = $state<ChannelStatus[]>([]);
  let error = $state('');
  let busy = $state(false);
  let doom = $state<Doom>(null);

  async function load() {
    try {
      rows = await api.channels();
      error = '';
    } catch (e) {
      error = (e as Error).message;
    }
  }
  $effect(() => {
    load();
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
        const next = await api.channelQR('whatsapp');
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
    const out = await run(api.channelAction<{ code: string }>('whatsapp', 'pair-phone', { phone }));
    if (out?.code) phoneCode = out.code;
  }

  // ---- grupo ----
  let groups = $state<{ id: string; name: string }[]>([]);
  let groupName = $state('');
  async function loadGroups() {
    groups = (await run(api.channelAction<{ id: string; name: string }[]>('whatsapp', 'groups'))) ?? [];
  }
  $effect(() => {
    if (waState === 'connected' && untrack(() => !groups.length)) untrack(loadGroups);
  });
  function chooseGroup(body: { id?: string; name?: string }) {
    run(api.channelAction('whatsapp', 'group', body));
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
    Por dónde entra al inbox lo que no se escribe en la app. Lo que llega se captura a nombre de
    quien enciende el canal, y dice quién lo mandó.
  </p>

  {#if error}<p class="set-err" role="alert">{error}</p>{/if}

  {#if wa}
    <div class="set-card">
      <div class="set-row head">
        <h3>🟢 WhatsApp</h3>
        <span class="state {wa.state}">{stateText[wa.state] ?? wa.state}</span>
        <Switch
          class="ml-auto"
          checked={wa.enabled}
          disabled={busy}
          onCheckedChange={(e: { checked: boolean }) => run(api.setChannel('whatsapp', e.checked))}>
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

      {#if wa.enabled && (wa.state === 'unlinked' || wa.state === 'pairing')}
        <div class="block">
          <h4>1 · Vincular la cuenta</h4>
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
              <button class="btn btn-sm preset-filled-primary-500" disabled={busy} onclick={() => run(api.channelAction('whatsapp', 'pair'))}>
                Vincular con QR
              </button>
              <span class="faint text-xs">o</span>
              <input class="input phone" placeholder="52 55 1234 5678" bind:value={phone} />
              <button class="btn btn-sm preset-tonal-surface" disabled={busy || !phone.trim()} onclick={pairPhone}>
                Pedir código
              </button>
            </div>
          {/if}
        </div>
      {/if}

      {#if wa.enabled && wa.state === 'connected'}
        <div class="block">
          <p class="help">
            Vinculado como <b>{extra('push_name') || 'la cuenta'}</b> {extra('account')}.
          </p>
          <h4>2 · El grupo que se escucha</h4>
          {#if extra('group_id')}
            <p>Escuchando <b>«{extra('group_name')}»</b>.</p>
          {:else}
            <p class="set-err">Todavía no hay grupo: no entra nada.</p>
          {/if}
          <div class="set-row">
            <input class="input grow" placeholder="Nombre del grupo, tal cual" bind:value={groupName} />
            <button class="btn btn-sm preset-filled-primary-500" disabled={busy || !groupName.trim()} onclick={() => chooseGroup({ name: groupName })}>
              Usar este grupo
            </button>
          </div>
          {#if groups.length}
            <p class="help">O elígelo de los grupos de la cuenta:</p>
            <ul class="groups">
              {#each groups as g (g.id)}
                <li>
                  <button class:on={g.id === extra('group_id')} onclick={() => chooseGroup({ id: g.id })}>{g.name || g.id}</button>
                </li>
              {/each}
            </ul>
          {/if}
          <button class="btn btn-sm preset-tonal-surface" disabled={busy} onclick={loadGroups}>Actualizar la lista</button>
        </div>
        <div class="set-row">
          <button
            class="btn btn-sm preset-tonal-surface ml-auto"
            disabled={busy}
            onclick={() =>
              (doom = {
                title: '¿Desvincular WhatsApp?',
                body: 'Se cierra la sesión de este dispositivo en la cuenta. Deja de entrar lo del grupo hasta que vuelvas a vincular.',
                verb: 'Desvincular',
                go: () => {
                  phoneCode = '';
                  groups = [];
                  run(api.channelAction('whatsapp', 'unlink'));
                },
              })}>
            Desvincular
          </button>
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
  .state { padding: 0.05rem 0.5rem; border-radius: 999px; background: var(--hover); color: var(--muted); font-size: 0.72rem; }
  .state.connected { background: color-mix(in oklab, var(--color-success-500) 22%, transparent); color: var(--text); }
  .state.error { background: color-mix(in oklab, var(--color-error-500) 18%, transparent); color: var(--text); }
  .help { margin: 0.4rem 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .detail { margin: 0.4rem 0; color: var(--muted); font-size: 0.82rem; }
  .block { display: flex; flex-direction: column; gap: 0.5rem; margin-top: 0.9rem; padding-top: 0.9rem; border-top: 1px solid var(--line); }
  .block h4 { margin: 0; font-size: 0.85rem; font-weight: 700; }
  .block p { margin: 0; font-size: 0.85rem; }
  .qr { width: 240px; height: 240px; border-radius: 10px; background: white; padding: 8px; image-rendering: pixelated; }
  .code { margin: 0; font-family: ui-monospace, monospace; font-size: 1.6rem; font-weight: 700; letter-spacing: 0.15em; }
  .phone { width: 12rem; }
  .grow { flex: 1; min-width: 12rem; }
  .groups { display: flex; flex-wrap: wrap; gap: 0.35rem; margin: 0; padding: 0; list-style: none; }
  .groups button { padding: 0.2rem 0.6rem; border: 1px solid var(--line); border-radius: 999px; font-size: 0.8rem; }
  .groups button.on { background: var(--hover); border-color: currentColor; font-weight: 600; }
</style>
