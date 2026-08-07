// Shared state for the right-click context menus. Single menu components live in
// Workspace and render from these; the cards and the canvas open them via show().
// Only one is ever open: opening either closes the other.
import type { BubbleView } from './types';

class BubbleMenu {
  open = $state(false);
  x = $state(0);
  y = $state(0);
  bubble = $state<BubbleView | null>(null);

  show(b: BubbleView, x: number, y: number): void {
    boardMenu.open = false;
    this.bubble = b;
    this.x = x;
    this.y = y;
    this.open = true;
  }

  hide(): void {
    this.open = false;
    this.bubble = null;
  }
}

// BoardMenu is the canvas-level menu: what you can do to the board itself,
// rather than to one bubble.
class BoardMenu {
  open = $state(false);
  x = $state(0);
  y = $state(0);

  show(x: number, y: number): void {
    bubbleMenu.open = false;
    bubbleMenu.bubble = null;
    this.x = x;
    this.y = y;
    this.open = true;
  }

  hide(): void {
    this.open = false;
  }
}

// WorkspaceMenu hangs off the ⋯ beside the project filter. A workspace is the
// outermost container — a Plane project — and until now the web treated it as a
// filter and nothing else, so it was the one tier you could not act on.
//
// It is ANCHORED to its button rather than to the pointer: it is opened by a
// click on a known element, not by a right-click somewhere on the canvas.
//
// Both edges of that button are kept, not just one. The button lives in the
// status bar at the BOTTOM of the window, so a menu placed under it opens off
// the bottom of the screen — invisible, with nothing to say it opened. The
// component picks the edge that fits.
class WorkspaceMenu {
  open = $state(false);
  x = $state(0);
  /** viewport y of the anchor's top edge */
  top = $state(0);
  /** viewport y of the anchor's bottom edge */
  bottom = $state(0);

  showAt(el: HTMLElement): void {
    bubbleMenu.open = false;
    bubbleMenu.bubble = null;
    boardMenu.open = false;
    const r = el.getBoundingClientRect();
    this.x = r.left;
    this.top = r.top;
    this.bottom = r.bottom;
    this.open = true;
  }

  hide(): void {
    this.open = false;
  }
}

export const bubbleMenu = new BubbleMenu();
export const boardMenu = new BoardMenu();
export const workspaceMenu = new WorkspaceMenu();
