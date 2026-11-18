import { ApiError, api } from '#lib/api.js';
import { cvSavesSettled, queueCvSave } from './cvSaveQueue';

/** What the workspace persists: the CV's title, template and document. */
export type CvDraft = Parameters<typeof api.updateCv>[1];

/**
 * The tailoring workspace's autosave (folded in from the old standalone CvEditor). There are
 * no Save buttons: any edit schedules a save 800ms later, resetting the timer on each
 * keystroke, and the lifecycle mirrors the old editor — 'idle' before the first change, then
 * saving → saved (or error).
 *
 * Construct during component initialisation: the debounce is an effect.
 */
export class CvAutosave {
  state = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  error = $state('');
  // A JSON snapshot of the last-persisted state; the effect compares against it to detect
  // real edits (and skip the initial load), and a save advances it on success.
  #lastSnapshot = '';
  #timer: ReturnType<typeof setTimeout> | null = null;

  /** `onSaved` runs after every save that landed. */
  constructor(
    private readonly cvId: () => string,
    private readonly draft: () => CvDraft,
    private readonly loaded: () => boolean,
    private readonly onSaved: () => void,
  ) {
    $effect(() => {
      if (!this.loaded()) return;
      if (this.#snapshot() === this.#lastSnapshot) return; // subscribes to the draft
      if (this.#timer) clearTimeout(this.#timer);
      this.#timer = setTimeout(() => {
        this.#timer = null;
        void this.#persist();
      }, 800);
    });
  }

  #snapshot = () => JSON.stringify(this.draft());

  get dirty() {
    return this.#snapshot() !== this.#lastSnapshot;
  }

  /** The draft as it stands is what the server holds (a load, a template the gallery saved). */
  accept() {
    this.#lastSnapshot = this.#snapshot();
  }

  // Saves go through this CV's queue (see cvSaveQueue), so a flush (agent turn, PDF export)
  // can wait for the one in flight: the timer is already cleared by then, and the server
  // would still hold the old document. #write() never rejects.
  #persist = () => queueCvSave(this.cvId(), () => this.#write());

  async #write() {
    const snap = this.#snapshot(); // capture NOW; edits during the round-trip re-trigger the effect
    this.state = 'saving';
    try {
      await api.updateCv(this.cvId(), this.draft());
      this.#lastSnapshot = snap;
      this.state = 'saved';
      this.onSaved();
    } catch (e) {
      this.state = 'error';
      this.error = e instanceof ApiError ? e.message : 'Could not save. Please try again.';
    }
  }

  /**
   * Write a pending human edit NOW rather than 800ms from now. Anything that changes the CV
   * server-side has to do this first, or the debounce fires afterwards and overwrites it. The
   * pending edit is queued before the wait, not after it: a debounce left running would queue
   * its save during the wait and the flush would return before that save landed.
   */
  flushPending = async () => {
    if (this.#timer) {
      clearTimeout(this.#timer);
      this.#timer = null;
      if (this.dirty) void this.#persist();
    }
    await cvSavesSettled(this.cvId());
  };

  /** The PDF is rendered from the saved CV, so a save that failed must stop the export rather
   *  than let it download the old one. Ordinary autosave failures keep showing in `state`. */
  flushForExport = async () => {
    await this.flushPending();
    // A failed or overtaken save leaves the document unsaved: write it again before the fetch.
    if (this.dirty) await this.#persist();
    if (this.dirty) throw new Error(this.error);
  };

  /** The workspace is going away: a pending edit is still written, fire-and-forget. */
  destroy() {
    if (this.#timer) clearTimeout(this.#timer);
    if (this.loaded() && this.dirty) {
      const cvId = this.cvId();
      const body = this.draft();
      void queueCvSave(cvId, () => api.updateCv(cvId, body)).catch(() => {});
    }
  }
}
