import { api } from '#lib/api.js';
import { markUndismissed } from '#lib/dismissedJobs.svelte.js';

/**
 * The pending "Job hidden — Undo" toast, or null. Set when a card is hidden; the toast owns
 * its auto-dismiss. Undo clears the slug's hidden mark (the card returns to the feed) and
 * confirms with the server; a failed undo is swallowed — the durable recovery path is
 * Activity → Hidden.
 */
export class HiddenJobUndo {
  pending = $state<{ slug: string } | null>(null);

  onHide = (slug: string) => {
    this.pending = { slug };
  };

  close = () => {
    this.pending = null;
  };

  undo = async () => {
    const pending = this.pending;
    if (!pending) return;
    markUndismissed(pending.slug);
    this.pending = null;
    try {
      await api.undismissJob(pending.slug);
    } catch {
      // Swallow: the job is already back in the feed optimistically, and Activity →
      // Hidden remains the durable way to manage hidden jobs.
    }
  };
}
