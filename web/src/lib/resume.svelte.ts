// The signed-in user's stored CV as the profile section reads it: whether one exists,
// the structured parse's status, and the candidate-owned contact block.
//
// It is a store rather than a per-page fetch because the profile's tabs are separate
// routes now, and three of them need the same answer — Settings (is there a CV to
// replace), Profile (the contact block itself) and the first-time set-up form. One
// GET /me/resume serves all of them instead of one per navigation.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the meta stays null for signed-out users.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { ResumeMeta } from '$lib/types';

class ResumeStore extends UserResource<ResumeMeta | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough.
  #meta = $state.raw<ResumeMeta | null>(null);
  // Optimistic: an upload stores the CV server-side before the re-fetch resolves (and
  // before any profile exists during set-up), so the uploaded state shows at once.
  #justUploaded = $state(false);

  get meta(): ResumeMeta | null {
    return this.#meta;
  }

  /** True once a CV is stored — including the moment right after an upload, before the
   *  re-fetch confirming it has resolved. */
  get present(): boolean {
    return this.#justUploaded || (this.#meta?.present ?? false);
  }

  protected load(): Promise<ResumeMeta | null> {
    return api.getResume();
  }

  protected apply(row: ResumeMeta | null) {
    this.#meta = row;
  }

  protected clearState() {
    this.#meta = null;
    this.#justUploaded = false;
  }

  /** Record that an upload succeeded: reflect it at once, then confirm from the server.
   *  One call rather than a flag and a re-fetch the caller has to remember to pair —
   *  both places that upload a CV want exactly this. */
  noteUpload(): void {
    this.#justUploaded = true;
    void this.refresh();
  }

  /** Re-fetch after a write (upload, contacts edit, parse retry). `ensureLoaded` would
   *  no-op once loaded, so a mutation has to say so explicitly. Best-effort: a failure
   *  leaves the previous copy in place rather than blanking the section. */
  async refresh(): Promise<void> {
    try {
      this.#meta = await api.getResume();
      this.markLoaded();
    } catch {
      // best-effort: keep whatever was last read.
    }
  }
}

export const resumeStore = new ResumeStore();
