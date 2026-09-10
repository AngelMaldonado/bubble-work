// Whether the markdown editor uses vim keybindings.
//
// A preference, not a setting: it belongs to the person, survives reloads, and
// is shared by every editor on screen — having one editor in vim mode and the
// one beside it not would be absurd.
const KEY = 'bubble.vim';

class VimPref {
  on = $state(false);

  constructor() {
    try {
      this.on = localStorage.getItem(KEY) === '1';
    } catch {
      // private mode, or storage disabled — the default is simply off
    }
  }

  toggle(): void {
    this.on = !this.on;
    try {
      localStorage.setItem(KEY, this.on ? '1' : '0');
    } catch {
      // not being able to remember it is not a reason to refuse the change
    }
  }
}

export const vimPref = new VimPref();
