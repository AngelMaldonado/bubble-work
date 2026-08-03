// Shared state for the bubble right-click context menu. A single menu component
// (in Workspace) renders from this; any BubbleCard opens it via show().
import type { BubbleView } from './types';

class BubbleMenu {
  open = $state(false);
  x = $state(0);
  y = $state(0);
  bubble = $state<BubbleView | null>(null);

  show(b: BubbleView, x: number, y: number): void {
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

export const bubbleMenu = new BubbleMenu();
