// What counts as a delivered result in the tailoring workspace, as analytics events. A seeded
// or opened CV is not one, and neither is a click on Download: only a completed agent turn that
// left a changed document on the server, and a PDF that actually arrived.
//
// The events carry the vacancy slug (public) and nothing from the CV. Who had the result is the
// analytics identity (`identifyUser`), so "first" and "later" results are read per account there
// rather than flagged here.

type Store = Pick<Storage, 'getItem' | 'setItem'>;

/** localStorage, or undefined where the browser blocks it (the result then cannot be
 *  deduplicated, so it is not counted). */
export function browserStore(): Store | undefined {
  try {
    return localStorage;
  } catch {
    return undefined;
  }
}

export interface OutcomeDeps {
  cvId: () => string;
  /** The vacancy slug; undefined in base mode, where there is no vacancy to tailor for. */
  slug: () => string | undefined;
  /** The CV document as last read from the server. */
  document: () => unknown;
  /** Where "this CV already produced its result" is remembered; undefined disables the event. */
  store?: Store | undefined;
  emit: (event: string, props: Record<string, unknown>) => void;
}

export class TailorOutcomes {
  readonly #d: OutcomeDeps;
  #before: string | null = null;
  #edited = false;
  // Turns that ended and await their turnEnded(): the host's end handler is async, and a queued
  // message can start the next turn before it runs, resetting the live fields above.
  readonly #ended: { before: string | null; edited: boolean }[] = [];

  constructor(deps: OutcomeDeps) {
    this.#d = { store: browserStore(), ...deps };
  }

  /** The chat's turn-active signal: a turn beginning is where the baseline is taken. */
  turnState(active: boolean) {
    if (active) this.turnStarted();
    else this.#ended.push({ before: this.#before, edited: this.#edited });
  }

  /** An agent turn began: remember the document it started from. */
  turnStarted() {
    this.#before = JSON.stringify(this.#d.document());
    this.#edited = false;
  }

  /** A cv_edit tool call succeeded during the current turn. */
  documentEdited() {
    this.#edited = true;
  }

  /** The turn ended and the workspace re-read the persisted document. `completed` is true only
   *  for a turn the server finished cleanly (not cut off, refused, errored or only caught up on
   *  after a dropped stream). Emits `tailor_result` at most once per CV. */
  turnEnded(completed: boolean) {
    const { before, edited } = this.#ended.shift() ?? { before: this.#before, edited: this.#edited };
    this.#before = null;
    this.#edited = false;
    const slug = this.#d.slug();
    if (!completed || !edited || before === null || slug === undefined) return;
    if (JSON.stringify(this.#d.document()) === before) return;
    // Storage that refuses writes cannot be deduplicated, and a double count is worse than a miss.
    const key = `fh:tailor-result:${this.#d.cvId()}`;
    try {
      if (!this.#d.store || this.#d.store.getItem(key) !== null) return;
      this.#d.store.setItem(key, '1');
    } catch {
      return;
    }
    this.#d.emit('tailor_result', { slug });
  }

  /** The PDF response arrived and was handed to the browser. */
  pdfExported() {
    const slug = this.#d.slug();
    this.#d.emit('cv_pdf_export', slug === undefined ? { kind: 'base' } : { kind: 'tailored', slug });
  }
}
