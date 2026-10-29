// The signed-in user's stored CV as the profile section reads it: whether one exists,
// the structured parse's status, and the candidate-owned contact block.
//
// It is a store rather than a per-page fetch because the profile's tabs are separate
// routes, and several of them need the same answer — Profile (the CV box, the contact
// block itself), Experience (education, languages, certifications) and the first-time
// set-up form. One GET /me/resume serves all of them instead of one per navigation.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the meta stays null for signed-out users.

import { api } from '#lib/api.js';
import { UserResource } from '#lib/userResource.svelte.js';
import type { CandidateContacts, ResumeMeta } from '#lib/types.js';

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

  /** Record a saved contact block: reflect it at once, then confirm from the server.
   *  Every editor of this block (contacts, CV summary, education) PUTs the WHOLE block
   *  built from `meta.contacts`, so until that copy is current a second save — from the
   *  same card or another one on the page — would send back the values the first save
   *  just replaced. The PUT's own answer is the new block, so it lands here before the
   *  re-fetch rather than after it. */
  noteContactsSaved(contacts: CandidateContacts): void {
    if (this.#meta) this.#meta = { ...this.#meta, contacts };
    void this.refresh();
  }
}

export const resumeStore = new ResumeStore();
