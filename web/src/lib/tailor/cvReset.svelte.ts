import { announce } from '#lib/announce.svelte.js';
import { ApiError, api } from '#lib/api.js';
import { askCvRefresh } from '#lib/cvRefreshDialog.svelte.js';
import { offerCvRefresh, TAILOR_REFRESH_MESSAGE } from '#lib/cvRefreshOffer.js';
import type { CvRecord } from '#lib/cv.js';
import { undoRun } from './autopilot';

/**
 * Resetting the workspace's CV to a fresh seed from the experience bank, from the History
 * tab's control or offered after a bank edit.
 */
export class CvReset {
  busy = $state(false);
  error = $state('');
  confirmOpen = $state(false);
  // The one condition that forbids replacing the whole document: another reset already in
  // flight, or any agent turn. All three write the same document, so a second writer loses
  // one side's work silently. Every entry point to a reset reads this, so none can drift.
  locked = $derived.by(() => this.busy || this.agentBusy());

  /** `agentBusy`: a turn or run is in flight. `hydrate` takes the reseeded record;
   *  `refetch` re-reads what hangs off the document. */
  constructor(
    private readonly cvId: () => string,
    private readonly agentBusy: () => boolean,
    private readonly flush: () => Promise<void>,
    private readonly hydrate: (rec: CvRecord) => void,
    private readonly refetch: () => void,
  ) {}

  apply = async () => {
    this.busy = true;
    this.error = '';
    try {
      await undoRun({
        flush: this.flush,
        undo: async () => this.hydrate(await api.reseedCv(this.cvId())),
        refetch: async () => this.refetch(),
      });
      announce('Changes reset');
    } catch (e) {
      this.error = e instanceof ApiError ? e.message : 'Could not reset changes.';
      // Shown in two places (the History tab and the Experience tab), spoken once here.
      announce(this.error, 'assertive');
    } finally {
      this.busy = false;
    }
  };

  ask = async () => {
    this.confirmOpen = true;
  };

  // A bank edit offers the same whole-document reset the History tab's control offers, so it
  // answers to the same guard. While that guard is up the offer is skipped rather than shown:
  // asking and then doing nothing reads as a broken control, and the next bank edit asks again.
  offerAfterBankEdit = () => {
    if (this.locked) return;
    void offerCvRefresh({ message: TAILOR_REFRESH_MESSAGE, apply: this.apply, confirm: askCvRefresh });
  };
}
