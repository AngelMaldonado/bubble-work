// The theme, applied before first paint by a script in index.html and kept in
// step here. Three states, not two: "system" is a real choice and the default.
export type Mode = 'system' | 'light' | 'dark';

const KEY = 'bubble.theme';

function apply(mode: Mode) {
  const dark =
    mode === 'dark' ||
    (mode === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.setAttribute('data-mode', dark ? 'dark' : 'light');
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
}

export const theme = $state({
  mode: (localStorage.getItem(KEY) as Mode) || 'system',
  set(mode: Mode) {
    this.mode = mode;
    localStorage.setItem(KEY, mode);
    apply(mode);
  },
});

apply(theme.mode);
// Following the system while it is the chosen mode: a laptop that switches at
// sunset should switch this too, without a reload.
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (theme.mode === 'system') apply('system');
});
