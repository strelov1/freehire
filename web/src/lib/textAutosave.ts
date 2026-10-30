// Autosave for a card of typed fields: save a second after typing stops, or at once when
// focus leaves the card, with an Undo back to what was saved before. One copy of the
// rules every such card needs, because each is easy to get subtly wrong (#3879):
//
// - Edits that have not been sent make the card's boxes newer than anything saved, so
//   while there are any (or a save is out) the card must not copy the saved value back
//   over them — `holding` says when. That is also what keeps an invalid value on screen
//   to be fixed rather than replaced.
// - A value that fails `valid` is not saved, and stays unsent.
// - Saving what is already saved is skipped, so leaving an untouched field saves nothing.

import type { SaveStatus } from '#lib/saveStatus.svelte.js';

const SAVE_AFTER_MS = 1000;

type Options<T> = {
  status: SaveStatus;
  /** The card's current boxes, normalised the way they would be saved. */
  read: () => T;
  /** What is saved now. */
  saved: () => T;
  /** Write a value; also used, with the earlier value, for Undo. */
  save: (value: T) => Promise<unknown>;
  /** Whether a value may be saved at all. */
  valid?: (value: T) => boolean;
  /** Told when `valid` refuses a value, on the timed save as well as on leaving the card,
   *  so a card that can mark the field at once does. */
  refused?: (value: T) => void;
};

export class TextAutosave<T> {
  #opts: Options<T>;
  #dirty = false;
  #timer: ReturnType<typeof setTimeout> | undefined;
  // The value of the save still on its way, if one is. Until it lands, `saved()` answers
  // with the value from before it — so that, not `saved()`, is what the boxes now differ
  // from: a box reverted mid-save must still be sent, and a box unchanged since need not.
  #sending: { value: T } | undefined;

  constructor(opts: Options<T>) {
    this.#opts = opts;
  }

  /** True while the boxes hold something newer than what is saved. */
  get holding(): boolean {
    return this.#dirty || this.#opts.status.saving;
  }

  /** Call on every edit. */
  edited(): void {
    this.#dirty = true;
    clearTimeout(this.#timer);
    this.#timer = setTimeout(() => this.commit(), SAVE_AFTER_MS);
  }

  /** Save now, if there is anything to save. Returns false when `valid` refused the
   *  value. `quiet` is for the card going away (see SaveStatus.run). */
  commit(quiet = false): boolean {
    clearTimeout(this.#timer);
    this.#timer = undefined;
    if (!this.#dirty) return true;
    const { status, read, saved, save, valid, refused } = this.#opts;
    const next = read();
    if (valid && !valid(next)) {
      refused?.(next);
      return false;
    }
    const before = saved();
    this.#dirty = false;
    if (same(next, this.#sending ? this.#sending.value : before)) return true;
    // Typing that lands during the save marks the card dirty again through `edited`, so
    // it stays unsent without anything to clear here.
    const sending = { value: next };
    this.#sending = sending;
    void status.run(() => save(next), { undo: () => save(before) }, quiet).then((ok) => {
      if (this.#sending === sending) this.#sending = undefined;
      if (!ok) this.#dirty = true;
    });
    return true;
  }
}

function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
