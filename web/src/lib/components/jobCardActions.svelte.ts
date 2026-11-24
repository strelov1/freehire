import { api } from '#lib/api.js';
import { isUnauthorized } from '#lib/api-core.js';
import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
import { markDismissed, markUndismissed } from '#lib/dismissedJobs.svelte.js';
import { ensureSavedLoaded, isSaved, markSaved, markUnsaved } from '#lib/savedJobs.svelte.js';
import { promptSignIn } from '#lib/signin.js';

/**
 * A job card's save and hide buttons. Both are optimistic over the shared sets (the saved
 * and dismissed jobs every card reads), confirmed with the server and rolled back on
 * failure; a signed-out click routes to sign-in instead. Both buttons are overlay siblings
 * of the card link — not descendants — so neither ever triggers the card's navigation.
 */
export class JobCardActions {
  // Guards against a double-click racing two requests for the same job.
  saving = $state(false);
  // Set by a save made from THIS card, so the "view it on your board" strip answers the
  // click that just happened rather than decorating every saved card in a feed.
  justSaved = $state(false);
  // Guards against a double-click firing two dismiss requests for the same job.
  hiding = $state(false);

  constructor(
    private readonly slug: () => string,
    private readonly unsave: () => (slug: string) => Promise<unknown>,
    private readonly onHide: () => ((slug: string) => void) | undefined,
    // Opt-in: a surface that wants to hear the session ending (Matches); others keep the silent rollback.
    private readonly onUnauthorized: () => (() => void) | undefined = () => undefined,
  ) {}

  /** Toggle the save mark: flip the shared set first so the bookmark fills instantly,
   *  then confirm (no auto-save after a sign-in it routed to). */
  toggleSave = async () => {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    if (this.saving) return;
    this.saving = true;
    const slug = this.slug();
    const userID = currentUser()?.id;
    let wasSaved = isSaved(slug);
    let changed = false;
    try {
      // Read the final set before changing it: a late initial load must never undo
      // the user's toggle, and a handoff to another account cancels this action.
      await ensureSavedLoaded();
      if (currentUser()?.id !== userID) return;
      wasSaved = isSaved(slug);
      this.justSaved = false;
      if (wasSaved) markUnsaved(slug);
      else markSaved(slug);
      changed = true;
      if (wasSaved) {
        await this.unsave()(slug);
      } else {
        await api.saveJob(slug);
        this.justSaved = true;
      }
    } catch (e) {
      if (currentUser()?.id !== userID) return;
      if (changed) {
        if (wasSaved) markSaved(slug);
        else markUnsaved(slug);
      }
      if (isUnauthorized(e)) this.onUnauthorized()?.();
    } finally {
      this.saving = false;
    }
  };

  /** Hide (dismiss) the job from the feed: the feed drops the card at once, and on success
   *  the slug goes to the feed (onHide) so it can surface an undo affordance. */
  hide = async () => {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    if (this.hiding) return;
    this.hiding = true;
    const slug = this.slug();
    const userID = currentUser()?.id;
    markDismissed(slug);
    try {
      await api.dismissJob(slug);
      if (currentUser()?.id === userID) this.onHide()?.(slug);
    } catch (e) {
      if (currentUser()?.id !== userID) return;
      markUndismissed(slug);
      if (isUnauthorized(e)) this.onUnauthorized()?.();
    } finally {
      this.hiding = false;
    }
  };
}
