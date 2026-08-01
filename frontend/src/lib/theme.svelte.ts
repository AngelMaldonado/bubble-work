// Theme: light / dark / system. The choice is persisted; "system" follows the
// OS preference live. The resolved mode is stamped on <html data-theme> so CSS
// can switch variables.
export type Theme = 'light' | 'dark' | 'system';

const KEY = 'bubble.theme';
const mql = window.matchMedia('(prefers-color-scheme: dark)');

class ThemeStore {
  choice = $state<Theme>((localStorage.getItem(KEY) as Theme) || 'system');

  get resolved(): 'light' | 'dark' {
    if (this.choice === 'system') return mql.matches ? 'dark' : 'light';
    return this.choice;
  }

  constructor() {
    this.apply();
    mql.addEventListener('change', () => {
      if (this.choice === 'system') this.apply();
    });
  }

  set(t: Theme): void {
    this.choice = t;
    localStorage.setItem(KEY, t);
    this.apply();
  }

  cycle(): void {
    const order: Theme[] = ['system', 'light', 'dark'];
    this.set(order[(order.indexOf(this.choice) + 1) % order.length]);
  }

  apply(): void {
    const mode = this.resolved;
    // data-mode drives our tokens; data-theme stays Skeleton's ("cerberus").
    document.documentElement.setAttribute('data-mode', mode);
    document.documentElement.style.colorScheme = mode;
  }

  get icon(): string {
    return this.choice === 'system' ? '🖥' : this.choice === 'dark' ? '🌙' : '☀️';
  }
}

export const theme = new ThemeStore();
