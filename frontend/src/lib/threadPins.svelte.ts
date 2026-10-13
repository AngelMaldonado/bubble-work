import { api, ApiError } from './api';

export type ThreadPin = { id: string; name: string; seq: number; slug: string; project: string };

class ThreadPins {
  ids = $state<string[]>([]);
  items = $state<ThreadPin[]>([]);
  loaded = $state(false);
  loading = $state(false);
  saving = $state(false);
  error = $state('');
  private owner = '';
  private pending: Promise<void> | null = null;

  load(): Promise<void> {
    const owner = api.me?.id ?? '';
    if (!owner) return Promise.resolve();
    if (this.owner !== owner) { this.ids = []; this.items = []; this.loaded = false; this.owner = owner; this.pending = null; }
    if (this.pending) return this.pending;
    this.loading = true;
    this.error = '';
    const pending = (async () => {
      try {
        const prefs = await api.prefs(true);
        const ids = [...new Set((prefs.thread_pins ?? []).filter((id) => typeof id === 'string' && /^[a-z0-9]{15}$/.test(id)))];
        const workspaces = await api.workspaces();
        const items = await Promise.all(ids.map(async (id): Promise<ThreadPin | null> => {
          try {
            const t = await api.threadRecord(id);
            const w = workspaces.find((w) => w.id === t.workspace);
            return w ? { id, name: t.name, seq: t.seq, slug: w.slug, project: w.name } : null;
          } catch (e) {
            // Borrado o ya no alcanzable: no conservar su nombre en otra copia.
            if (e instanceof ApiError && e.status === 404) return null;
            throw e;
          }
        }));
        if (api.me?.id !== owner) return;
        this.ids = ids;
        this.items = items.filter((t): t is ThreadPin => !!t);
        this.loaded = true;
      } catch (e) { if (this.owner === owner) this.error = (e as Error).message; }
      finally { if (this.owner === owner) this.loading = false; }
    })();
    this.pending = pending;
    pending.finally(() => { if (this.pending === pending) this.pending = null; });
    return pending;
  }

  async toggle(id: string) {
    if (this.saving || this.loading || !this.loaded) return;
    this.saving = true;
    this.error = '';
    try {
      const next = this.ids.includes(id) ? this.ids.filter((x) => x !== id) : [...this.ids, id];
      await api.setPrefs({ thread_pins: next });
      await this.load();
    } catch (e) { this.error = (e as Error).message; }
    finally { this.saving = false; }
  }
}

export const threadPins = new ThreadPins();
