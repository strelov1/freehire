import { api } from '#lib/api.js';
import type { RevisionView } from '#lib/generated/contracts.js';
import { undoRun } from './autopilot';

// The feed is loaded beside the CV and refreshed after anything that writes: a save, an
// agent turn, an undo. `pinnedRevision` is what the preview underlines; it lives here rather
// than in the panel because the highlight belongs to the document, not to the tab.
//
// The tailoring workspace's revision history for the CV `cvId` names: the feed, the
// revision the preview underlines, and undoing one revision or a whole run. `flush` writes
// the editor's pending save first; `refetch` re-reads everything an undo changed.
export class CvRevisions {
  readonly #cvId: () => string;
  readonly #flush: () => Promise<void>;
  readonly #refetch: () => Promise<void>;

  list = $state<RevisionView[]>([]);
  pinned = $state<RevisionView | null>(null);

  load = async (highlightNewestRun = false) => {
    const cvId = this.#cvId();
    if (!cvId) return;
    try {
      this.list = await api.listCvRevisions(cvId);
      // After a run, what it changed is underlined without being asked: the candidate's first
      // question is "what did it do to my CV", and the answer is on the page.
      if (highlightNewestRun) {
        this.pinned = this.list.find((r) => r.actor === 'agent' && !r.reverted) ?? this.pinned;
      }
    } catch {
      // The history is an accessory read: a workspace that cannot list it still edits.
      this.list = [];
    }
  };

  // Undoing goes through undoRun's ordering for the reason it was written: the document is
  // saved on a debounce, so undoing without flushing the pending save first lets the timer
  // write the old text straight back a second later — the undo appears to work and then
  // silently reverses itself. The re-read comes last, and only on success.
  undo = async (revision: RevisionView) => {
    await undoRun({
      flush: this.#flush,
      undo: () => api.undoCvRevision(this.#cvId(), revision.id).then(() => undefined),
      refetch: this.#refetch,
    });
  };

  undoRun = async (batchId: string) => {
    await undoRun({
      flush: this.#flush,
      undo: () => api.undoCvRevisionRun(this.#cvId(), batchId).then(() => undefined),
      refetch: this.#refetch,
    });
  };

  constructor(cvId: () => string, flush: () => Promise<void>, refetch: () => Promise<void>) {
    this.#cvId = cvId;
    this.#flush = flush;
    this.#refetch = refetch;
  }
}
