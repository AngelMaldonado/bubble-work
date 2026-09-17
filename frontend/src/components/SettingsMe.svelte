<script lang="ts">
  // Mi usuario: el nombre con el que se firma lo que escribo, mi cara y mi
  // contraseña. El correo se ve y no se cambia: hacerlo bien pide verificar el
  // nuevo por correo, y esta instancia no manda correos.
  import { api, avatarUrl, type Person } from '../lib/api';
  import { limited } from '../lib/limits.svelte';

  let { me, onme }: { me: Person; onme?: (p: Person) => void } = $props();

  // ---- nombre ----
  let name = $state('');
  let nameSeeded = '';
  $effect(() => {
    // Sembrado cuando cambia la persona, no en cada render: escribir aquí no
    // debe deshacerse porque arriba se volvió a pintar.
    if (me.id !== nameSeeded) {
      nameSeeded = me.id;
      name = me.display_name ?? '';
    }
  });
  let savingName = $state(false);
  let nameMsg = $state('');
  let nameErr = $state('');
  async function saveName() {
    savingName = true;
    nameMsg = nameErr = '';
    try {
      onme?.(await api.updateMe({ display_name: name.trim() }));
      nameMsg = 'Guardado.';
    } catch (e) {
      nameErr = (e as Error).message;
    } finally {
      savingName = false;
    }
  }

  // ---- avatar ----
  let fileInput = $state<HTMLInputElement | null>(null);
  let avatarErr = $state('');
  let uploading = $state(false);
  async function upload(file: File | undefined) {
    if (!file) return;
    avatarErr = '';
    if (file.size > 2 * 1024 * 1024) {
      avatarErr = 'La imagen pasa de 2 MB.';
      return;
    }
    uploading = true;
    try {
      const form = new FormData();
      form.append('avatar', file);
      onme?.(await api.updateMe(form));
    } catch (e) {
      avatarErr = (e as Error).message;
    } finally {
      uploading = false;
      if (fileInput) fileInput.value = '';
    }
  }
  async function removeAvatar() {
    avatarErr = '';
    uploading = true;
    try {
      onme?.(await api.updateMe({ avatar: null }));
    } catch (e) {
      avatarErr = (e as Error).message;
    } finally {
      uploading = false;
    }
  }

  // ---- contraseña ----
  let oldPass = $state('');
  let newPass = $state('');
  let again = $state('');
  let savingPass = $state(false);
  let passMsg = $state('');
  let passErr = $state('');
  const passProblem = $derived(
    !newPass
      ? ''
      : newPass.length < 8
        ? 'al menos 8 caracteres'
        : again && again !== newPass
          ? 'no coinciden'
          : '',
  );
  async function savePass(e: SubmitEvent) {
    e.preventDefault();
    if (passProblem || !oldPass || newPass !== again) return;
    savingPass = true;
    passMsg = passErr = '';
    try {
      onme?.(await api.changePassword(oldPass, newPass));
      oldPass = newPass = again = '';
      passMsg = 'Contraseña cambiada. Tus otras sesiones se cerraron.';
    } catch (e) {
      passErr = (e as Error).message;
    } finally {
      savingPass = false;
    }
  }

  const initial = $derived((me.display_name || me.email || '?')[0]?.toUpperCase());
</script>

<section class="set-section">
  <div class="set-card">
    <h3>Perfil</h3>
    <div class="profile">
      <div class="avatar-col">
        <button
          class="avatar"
          onclick={() => fileInput?.click()}
          disabled={uploading}
          title="Cambiar la foto"
          aria-label="Cambiar la foto">
          {#if me.avatar}
            <img src={avatarUrl(me, '160x160')} alt="" />
          {:else}
            <span>{initial}</span>
          {/if}
        </button>
        <input
          bind:this={fileInput}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          hidden
          onchange={(e) => upload(e.currentTarget.files?.[0])} />
        <div class="set-row">
          <button class="link" onclick={() => fileInput?.click()} disabled={uploading}>
            {uploading ? 'subiendo…' : me.avatar ? 'cambiar' : 'poner foto'}
          </button>
          {#if me.avatar}<button class="link" onclick={removeAvatar} disabled={uploading}>quitar</button>{/if}
        </div>
      </div>

      <div class="fields">
        <label class="field">
          <span class="name">Correo</span>
          <input class="input" value={me.email} readonly />
          <span class="help">No se cambia desde aquí: cambiarlo pide verificar el nuevo por correo.</span>
        </label>
        <label class="field">
          <span class="name">Nombre visible</span>
          <span class="set-row">
            <input
              class="input grow"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-bwignore data-form-type="other"
              placeholder={me.email}
              bind:value={name}
              {@attach limited('users.display_name')}
              onkeydown={(e) => e.key === 'Enter' && saveName()} />
            <button
              class="btn btn-sm preset-filled-primary-500"
              onclick={saveName}
              disabled={savingName || name.trim() === (me.display_name ?? '')}>
              {savingName ? '…' : 'Guardar'}
            </button>
          </span>
          <span class="help">Con él se firma lo que escribes y se te pone a cargo. Vacío, se usa tu correo.</span>
        </label>
        <p class="faint text-xs">
          Rol: <b>{me.role === 'lead' ? 'lead global' : 'member'}</b>
        </p>
        {#if nameMsg}<span class="set-ok">{nameMsg}</span>{/if}
        {#if nameErr}<p class="set-err" role="alert">{nameErr}</p>{/if}
        {#if avatarErr}<p class="set-err" role="alert">{avatarErr}</p>{/if}
      </div>
    </div>
  </div>

  <form class="set-card" onsubmit={savePass}>
    <h3>Contraseña</h3>
    <div class="fields">
      <label class="field">
        <span class="name">Actual</span>
        <input class="input" type="password" autocomplete="current-password" bind:value={oldPass} />
      </label>
      <label class="field">
        <span class="name">Nueva</span>
        <input class="input" type="password" autocomplete="new-password" bind:value={newPass} />
      </label>
      <label class="field">
        <span class="name">Otra vez</span>
        <input class="input" type="password" autocomplete="new-password" bind:value={again} />
      </label>
      <p class="help">
        Cambiarla cierra todas tus sesiones abiertas, y también la de un agente que use tu
        sesión en vez de un token propio. Los tokens de <b>Tokens</b> siguen valiendo.
      </p>
      <div class="set-row">
        <button
          class="btn btn-sm preset-filled-primary-500"
          type="submit"
          disabled={savingPass || !oldPass || !newPass || newPass !== again || !!passProblem}>
          {savingPass ? 'cambiando…' : 'Cambiar contraseña'}
        </button>
        {#if passProblem}<span class="set-err">{passProblem}</span>{/if}
        {#if passMsg}<span class="set-ok">{passMsg}</span>{/if}
      </div>
      {#if passErr}<p class="set-err" role="alert">{passErr}</p>{/if}
    </div>
  </form>
</section>

<style>
  .profile { display: flex; flex-wrap: wrap; gap: 1.5rem; }
  .avatar-col { display: flex; flex-direction: column; align-items: center; gap: 0.4rem; }
  .avatar {
    display: grid;
    place-content: center;
    width: 96px;
    height: 96px;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--hover);
    color: var(--muted);
    font-size: 2.2rem;
    font-weight: 700;
    cursor: pointer;
  }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
  .avatar:hover { outline: 2px solid var(--line); }
  .link { color: var(--muted); font-size: 0.8rem; }
  .link:hover { color: var(--text); text-decoration: underline; }
  .fields { display: flex; flex: 1; flex-direction: column; gap: 0.85rem; min-width: 16rem; }
  .field { display: flex; flex-direction: column; gap: 0.3rem; }
  .name { font-size: 0.85rem; font-weight: 600; }
  .help { margin: 0; color: var(--faint); font-size: 0.8rem; line-height: 1.45; }
  .grow { flex: 1; min-width: 10rem; }
  input[readonly] { opacity: 0.75; }
</style>
