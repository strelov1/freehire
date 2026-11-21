import { prefersReducedMotion } from 'svelte/motion';

// How far a card must be dragged before the gesture counts as a decision.
export const COMMIT_PX = 100;
// How long a judged card takes to fly off-screen before the deck drops it.
export const FLY_MS = 300;
// Travel needed to reveal a gesture's axis.
export const AXIS_LOCK_PX = 8;

export type SwipeKind = 'save' | 'dismiss';

// The bits of a PointerEvent the gesture reads, so unit tests need no DOM.
export interface SwipePointer {
  clientX: number;
  clientY: number;
  pointerId: number;
  target: EventTarget | { closest(selector: string): unknown } | null;
  currentTarget: EventTarget | { setPointerCapture(id: number): void } | null;
}

// Drag state for one swipe card (touch + mouse). The card both swipes horizontally and
// scrolls its content vertically, so the axis is locked on first movement: a mostly
// vertical drag is left to native scroll, a horizontal one drives the swipe. Pointer
// capture is deferred until the axis is x - capturing up front would swallow every
// vertical scroll.
export class SwipeGesture {
  dragX = $state(0);
  dragging = $state(false);
  // Mid fly-off: guards re-entry and drives the fade-out.
  exiting = $state(false);

  // The card transitions (springs back or flies off) whenever it isn't tracking the finger.
  animate = $derived(!this.dragging);
  // Tilt tracks the drag, capped so the fly-off doesn't spin; never under reduced motion.
  rotation = $derived(prefersReducedMotion.current ? 0 : Math.max(-18, Math.min(18, this.dragX / 24)));
  opacity = $derived(this.exiting ? 0 : 1);
  // Stamps fade in with distance, reaching full at the commit point.
  stampOpacity = $derived(Math.min(1, Math.abs(this.dragX) / COMMIT_PX));

  #startX = 0;
  #startY = 0;
  #axis: 'none' | 'x' | 'y' = 'none';
  #pointer = -1;

  down = (e: SwipePointer) => {
    if (this.exiting) return;
    // Clicks on links/buttons inside the card go through - capturing would swallow them.
    if ((e.target as Element | null)?.closest('a, button')) return;
    this.#startX = e.clientX;
    this.#startY = e.clientY;
    this.#axis = 'none';
    this.#pointer = e.pointerId;
  };

  move = (e: SwipePointer) => {
    if (e.pointerId !== this.#pointer || this.#axis === 'y') return;
    const dx = e.clientX - this.#startX;
    const dy = e.clientY - this.#startY;
    if (this.#axis === 'none') {
      const adx = Math.abs(dx);
      const ady = Math.abs(dy);
      if (adx < AXIS_LOCK_PX && ady < AXIS_LOCK_PX) return;
      if (ady > adx) {
        this.#axis = 'y';
        return;
      }
      this.#axis = 'x';
      this.dragging = true;
      (e.currentTarget as Element | null)?.setPointerCapture(e.pointerId);
    }
    this.dragX = dx;
  };

  up = (onCommit: (kind: SwipeKind) => void) => {
    this.#pointer = -1;
    this.#axis = 'none';
    if (!this.dragging) return;
    if (this.dragX > COMMIT_PX) onCommit('save');
    else if (this.dragX < -COMMIT_PX) onCommit('dismiss');
    else this.settle();
  };

  // Fly the card off toward its side. Under reduced motion it stays put and only fades.
  fling(kind: SwipeKind) {
    this.exiting = true;
    this.dragging = false;
    if (!prefersReducedMotion.current) this.dragX = (kind === 'save' ? 1 : -1) * (window.innerWidth + 200);
  }

  // Back to rest (spring-back, or the next card mounting).
  settle() {
    this.dragX = 0;
    this.dragging = false;
    this.exiting = false;
  }

  reset() {
    this.settle();
    this.#pointer = -1;
    this.#axis = 'none';
  }
}
