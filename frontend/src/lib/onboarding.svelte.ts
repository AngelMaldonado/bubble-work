import { api } from './api';

// La bienvenida: se abre UNA vez, a quien no tiene `onboarded_at` en sus
// preferencias. Quien ya existía cuando salió quedó marcado por la migración
// 1788546000_onboarded, así que sólo la ve quien llega después.
//
// Se lee con `strict`: si la lectura falla, no se abre. Una red mala no es
// razón para volver a explicarle todo a alguien cada mañana.
class Onboarding {
  open = $state(false);
  /** Vista otra vez desde Ajustes: no se bloquea nada ni se crea un token solo. */
  replaying = $state(false);
  owner = '';
  async check() {
    const owner = api.me?.id ?? '';
    if (!owner || this.owner === owner) return;
    this.owner = owner;
    try {
      const prefs = await api.prefs(true);
      if (api.me?.id === owner) {
        this.replaying = false;
        this.open = !prefs.onboarded_at;
      }
    } catch {
      this.open = false;
    }
  }
  /** Terminar o saltarla es lo mismo: ya la vio. */
  async done() {
    this.open = false;
    await api.setPrefs({ onboarded_at: new Date().toISOString() }).catch(() => {});
  }
  /** Volver a verla, desde Ajustes → Yo. */
  replay() {
    this.replaying = true;
    this.open = true;
  }
}
export const onboarding = new Onboarding();
