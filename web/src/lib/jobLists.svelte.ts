// The signed-in user's job lists — named sets of specific jobs, independent of the
// single-flag "save" star. The list is read once from GET /api/v1/me/lists;
// create/update/delete/add/remove/share/unshare call the API and keep the local list
// in sync, newest-updated-first, so the account page and the job-card "Add to list"
// control update without a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and
// the list stays empty for signed-out users. Mutations surface API errors to the
// caller (a duplicate name or the per-user cap is a 409) so the UI can show them.

import { api } from '$lib/api';
import { UserResource } from '$lib/userResource.svelte';
import type { JobList } from '$lib/types';

class JobLists extends UserResource<JobList[]> {
  // Reassigned (never mutated in place) on every change, so $state.raw is enough
  // and readers ($derived in the component) re-run on each new array.
  #items = $state.raw<JobList[]>([]);

  get items(): JobList[] {
    return this.#items;
  }

  protected load(): Promise<JobList[]> {
    return api.listJobLists();
  }

  protected apply(rows: JobList[]) {
    this.#items = rows;
  }

  protected clearState() {
    this.#items = [];
  }

  /** Create a list; prepend it (newest-first). Throws on a duplicate name or the
   *  per-user cap (the caller shows the error). */
  async create(name: string, description = ''): Promise<JobList> {
    const row = await api.createJobList(name, description);
    this.#items = [row, ...this.#items];
    return row;
  }

  /** Overwrite a list's name and/or description; move it to the front (it is now the
   *  most recently updated, matching the server's ordering). */
  async update(id: number, patch: { name?: string; description?: string }): Promise<JobList> {
    const row = await api.updateJobList(id, patch);
    this.#items = [row, ...this.#items.filter((l) => l.id !== id)];
    return row;
  }

  /** Delete a list and drop it from the list. */
  async remove(id: number): Promise<void> {
    await api.deleteJobList(id);
    this.#items = this.#items.filter((l) => l.id !== id);
  }

  /** Add a job to a list (by its public slug) and bump its job_count in place. */
  async addJob(id: number, jobSlug: string): Promise<void> {
    await api.addJobToList(id, jobSlug);
    this.#items = this.#items.map((l) => (l.id === id ? { ...l, job_count: l.job_count + 1 } : l));
  }

  /** Remove a job from a list (by its public slug) and drop its job_count in place
   *  (never below 0 — removing an absent job is idempotent server-side too). */
  async removeJob(id: number, jobSlug: string): Promise<void> {
    await api.removeJobFromList(id, jobSlug);
    this.#items = this.#items.map((l) =>
      l.id === id ? { ...l, job_count: Math.max(0, l.job_count - 1) } : l,
    );
  }

  /** Publish a list and replace it in place (keeping its position, so toggling share
   *  doesn't reorder the list). Returns the updated list with its slug. */
  async share(id: number): Promise<JobList> {
    const row = await api.shareJobList(id);
    this.#items = this.#items.map((l) => (l.id === id ? row : l));
    return row;
  }

  /** Make a shared list private again and clear its slug in place. */
  async unshare(id: number): Promise<void> {
    await api.unshareJobList(id);
    this.#items = this.#items.map((l) => (l.id === id ? { ...l, public_slug: '' } : l));
  }
}

export const jobLists = new JobLists();
