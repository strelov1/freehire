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
import { nextResumePollDelayMs } from '#lib/onboardingResumeWait.js';
import { serialQueue } from '#lib/serialQueue.js';
import { UserResource } from '#lib/userResource.svelte.js';
import type { CandidateContacts, ResumeMeta } from '#lib/types.js';

class ResumeStore extends UserResource<ResumeMeta | null> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough.
  #meta = $state.raw<ResumeMeta | null>(null);
  // Optimistic: an upload stores the CV server-side before the re-fetch resolves (and
  // before any profile exists during set-up), so the uploaded state shows at once.
  #justUploaded = $state(false);
  // Contact-block saves go one at a time: see saveContacts.
  #contactsQueue = serialQueue();

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

  /** Follow the background parse an upload started until it settles. Resolves true once
   *  it has written its result — the caller's cue to re-read what the parse feeds — and
   *  false when it failed, the user signed out, or it outlasted the wait. The waits are
   *  onboarding's (see onboardingResumeWait.ts): giving up is not failing, the parse goes
   *  on server-side and its result is there on the next visit. */
  async waitForParse(): Promise<boolean> {
    const generation = this.generation;
    for (let attempt = 0; ; attempt++) {
      let status: string | undefined;
      try {
        // oxlint-disable-next-line no-await-in-loop -- a backoff poll is sequential by design
        // A read superseded by another says nothing either: the copy held may predate it.
        if (!(await this.reload())) status = 'pending';
        else if (!this.#meta?.present) return false;
        else status = this.#meta.parse_status;
      } catch (err) {
        // A failed read says nothing about the parse; the copy held is from before it.
        console.warn('resume: parse poll failed', err);
        status = 'pending';
      }
      if (generation !== this.generation) return false;
      if (status !== 'pending') return status !== 'failed';
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) return false;
      // oxlint-disable-next-line no-await-in-loop -- the wait between reads is the point
      await new Promise((resolve) => setTimeout(resolve, delay));
      if (generation !== this.generation) return false;
    }
  }

  /** Save part of the candidate-owned contact block and return the whole saved block.
   *  PUT /me/resume/contacts replaces the WHOLE block, and three cards write it (contacts,
   *  CV summary, education), so two saves a moment apart — a field left in one card while
   *  another card's save is still out — would both be built from the same copy and the
   *  second would put back what the first replaced. So saves queue here, and each is
   *  built from the block the one before it saved. The re-read that follows (the parse
   *  view `structured` shows the owned values merged in) is part of the same job, so a
   *  slower read can't land after a newer save and hand the next one a stale copy. */
  saveContacts(changes: Partial<CandidateContacts>): Promise<CandidateContacts> {
    return this.#contactsQueue(async () => {
      const saved = await api.putResumeContacts({ ...this.#meta?.contacts, ...changes });
      if (this.#meta) this.#meta = { ...this.#meta, contacts: saved };
      await this.refresh();
      return saved;
    });
  }
}

export const resumeStore = new ResumeStore();
