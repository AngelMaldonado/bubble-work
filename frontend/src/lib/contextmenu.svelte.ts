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

export const bubbleMenu = new BubbleMenu();
export const boardMenu = new BoardMenu();
