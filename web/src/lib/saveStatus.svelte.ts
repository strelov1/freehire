// The save state of one autosaving control: saving, saved (for a moment), or failed with
// a retry. Every autosaving card on the profile reports through one of these and draws it
// with SaveStatus.svelte, so "it worked" looks and sounds the same everywhere instead of
// each card inventing its own (see freehire#3879).

import { ApiError } from '#lib/api.js';
import { announce } from '#lib/announce.svelte.js';
import { locale } from '#lib/i18n/currentLocale.svelte.js';
import { t } from '#lib/i18n/t.js';
import { messages } from './components/SaveStatus.messages';

type SaveState = 'idle' | 'saving' | 'saved' | 'error';

/** How to report one write: what to say instead of the generic "Saved" / "Couldn't
 *  save." (e.g. which skill was added, or which one failed), and how to reverse it. A
 *  write that can be undone shows "Saved · Undo" for long enough to reach the button. */
type SaveOptions = { saved?: string; failed?: string; undo?: () => Promise<unknown> };

const SAVED_FOR_MS = 1500;
const UNDOABLE_FOR_MS = 6000;

export class SaveStatus {
  state = $state<SaveState>('idle');
  /** The failure text: the caller's wording, else the server's own message, else null
   *  for the generic one (resolved at render time, so it follows a locale change). */
  failure = $state<string | null>(null);

  #last: { write: () => Promise<unknown>; options: SaveOptions } | null = null;
  #undo = $state<(() => Promise<unknown>) | null>(null);
  #timer: ReturnType<typeof setTimeout> | undefined;
  #seq = 0;

  get saving(): boolean {
    return this.state === 'saving';
  }

  /** Whether "Saved" carries an Undo right now. */
  get canUndo(): boolean {
    return this.state === 'saved' && this.#undo !== null;
  }

  /** Run one write and report it. Resolves whether it saved; never rejects. When writes
   *  overlap, only the newest one reports, so an older answer landing late can't say
   *  "Saved" over a newer failure. A card whose writes must not overlap (a toggle built
   *  from the current list) checks `saving` first. `quiet` saves without showing or
   *  saying anything — for a write flushed as the control goes away, when a "Saved"
   *  would describe something no longer on screen. */
  async run(write: () => Promise<unknown>, options: SaveOptions = {}, quiet = false): Promise<boolean> {
    this.#last = { write, options };
    if (quiet) {
      return write().then(
        () => true,
        (e: unknown) => {
          console.warn('save failed', e);
          return false;
        },
      );
    }
    const seq = ++this.#seq;
    clearTimeout(this.#timer);
    this.state = 'saving';
    this.failure = null;
    this.#undo = null;
    const s = t(messages, locale());
    try {
      await write();
      if (seq !== this.#seq) return true;
      this.state = 'saved';
      this.#undo = options.undo ?? null;
      announce(options.saved ?? s.saved);
      this.#timer = setTimeout(
        () => {
          if (this.state === 'saved') this.state = 'idle';
          this.#undo = null;
        },
        options.undo ? UNDOABLE_FOR_MS : SAVED_FOR_MS,
      );
      return true;
    } catch (e) {
      // Logged before the newest-write check: a superseded or quiet failure is shown
      // nowhere, and must still leave a trace.
      console.warn('save failed', e);
      if (seq !== this.#seq) return false;
      this.state = 'error';
      this.failure = options.failed ?? (e instanceof ApiError ? e.message : null);
      announce(this.failure ?? s.failed, 'assertive');
      return false;
    }
  }

  /** Run the last write again, after a failure. */
  retry(): Promise<boolean> {
    if (!this.#last) return Promise.resolve(false);
    return this.run(this.#last.write, this.#last.options);
  }

  /** Reverse the write that just saved. Reported like any write, without an Undo of its
   *  own: undoing the undo is just editing again. */
  undo(): Promise<boolean> {
    const undo = this.#undo;
    if (!undo) return Promise.resolve(false);
    return this.run(undo);
  }
}
