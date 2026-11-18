import { fitToWidthZoom } from '#lib/cvGeometry.js';

const SCROLLER_PADDING = 48; // the scroller's p-6, both sides

const clampZoom = (z: number) => Math.min(1.5, Math.max(0.5, Math.round(z * 10) / 10));

/**
 * The tailoring workspace's centre preview zoom. It opens fitted to the column's width, so
 * the whole sheet is readable whatever the window or the side panels leave; the + and −
 * buttons then step in 10% from there (50–150%), and the percentage puts it back to fit.
 * A fixed 80% start used to promise this and overflowed the column at 1440px with both
 * panels open.
 */
export class PreviewZoom {
  /** The preview column's width, bound from the scroller. */
  width = $state(0);
  fitMode = $state(true);
  #manual = $state(0.8);
  readonly #fit = $derived(fitToWidthZoom(this.width - SCROLLER_PADDING));
  readonly zoom = $derived(this.fitMode ? this.#fit : this.#manual);
  readonly zoomPct = $derived(Math.round(this.zoom * 100));

  #set(z: number) {
    this.#manual = clampZoom(z);
    this.fitMode = false;
  }

  zoomOut() {
    this.#set(this.zoom - 0.1);
  }

  zoomIn() {
    this.#set(this.zoom + 0.1);
  }
}
