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

/** What to say about one write instead of the generic "Saved" / "Couldn't save." —
 *  e.g. which skill was added, or which one failed. */
type SaveWording = { saved?: string; failed?: string };

const SAVED_FOR_MS = 1500;

export class SaveStatus {
  state = $state<SaveState>('idle');
  /** The failure text: the caller's wording, else the server's own message, else null
   *  for the generic one (resolved at render time, so it follows a locale change). */
  failure = $state<string | null>(null);

  #last: { write: () => Promise<unknown>; wording: SaveWording } | null = null;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #seq = 0;

  get saving(): boolean {
    return this.state === 'saving';
  }

  /** Run one write and report it. Resolves whether it saved; never rejects. When writes
   *  overlap, only the newest one reports, so an older answer landing late can't say
   *  "Saved" over a newer failure. A card whose writes must not overlap (a toggle built
   *  from the current list) checks `saving` first. `quiet` saves without showing or
   *  saying anything — for a write flushed as the control goes away, when a "Saved"
   *  would describe something no longer on screen. */
  async run(write: () => Promise<unknown>, wording: SaveWording = {}, quiet = false): Promise<boolean> {
    this.#last = { write, wording };
    if (quiet) return write().then(() => true, () => false);
    const seq = ++this.#seq;
    clearTimeout(this.#timer);
    this.state = 'saving';
    this.failure = null;
    const s = t(messages, locale());
    try {
      await write();
      if (seq !== this.#seq) return true;
      this.state = 'saved';
      announce(wording.saved ?? s.saved);
      this.#timer = setTimeout(() => {
        if (this.state === 'saved') this.state = 'idle';
      }, SAVED_FOR_MS);
      return true;
    } catch (e) {
      if (seq !== this.#seq) return false;
      this.state = 'error';
      this.failure = wording.failed ?? (e instanceof ApiError ? e.message : null);
      announce(this.failure ?? s.failed, 'assertive');
      return false;
    }
  }

  /** Run the last write again, after a failure. */
  retry(): Promise<boolean> {
    if (!this.#last) return Promise.resolve(false);
    return this.run(this.#last.write, this.#last.wording);
  }
}
