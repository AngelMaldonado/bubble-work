import { api } from './api';
import { DEFAULT_WEEK_START, validWeekStart } from './week';

// El día en que empieza la semana de quien mira, guardado en sus preferencias
// (`prefs.value.week_start`). Lo leen el roadmap, el calendario del planeador y
// los selectores de fecha. Mientras no se carga, el lunes de siempre.
class WeekStart {
  owner = '';
  day = $state(DEFAULT_WEEK_START);
  loaded = false;
  async load() {
    const owner = api.me?.id ?? '';
    if (!owner) return;
    if (this.owner === owner && this.loaded) return;
    if (this.owner !== owner) {
      this.owner = owner;
      this.day = DEFAULT_WEEK_START;
      this.loaded = false;
    }
    const prefs = await api.prefs(true);
    if (api.me?.id !== owner || this.owner !== owner) return;
    this.day = validWeekStart(prefs.week_start);
    this.loaded = true;
  }
  async set(day: number) {
    const owner = api.me?.id;
    if (!owner) return;
    const next = validWeekStart(day);
    await api.setPrefs({ week_start: next });
    if (api.me?.id === owner) this.day = next;
  }
}
export const weekStart = new WeekStart();
