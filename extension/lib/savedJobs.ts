// Tracks which jobs the signed-in user has saved, so the match card can render
// its Save button already filled without recording a view per job. A small port
// of web/src/lib/savedJobs.svelte.ts's cross-referencing: the set loads once
// from GET /me/tracking/saved (side-effect-free) and is kept in sync locally by
// the panel's own save/unsave clicks, same as the web app's markSaved/markUnsaved.

import { listSavedSlugs } from './freehire';

let loaded = false;
let loading: Promise<void> | null = null;
const slugs = new Set<string>();

/** Loads the saved set once per panel session; a second call while loading or
 *  after success is a no-op. A failed load leaves the set empty — nothing shows
 *  filled, which is the correct degraded state, not a retry loop. */
export function ensureSavedLoaded(token: string): Promise<void> {
  if (loaded) return Promise.resolve();
  if (!loading) {
    loading = listSavedSlugs(token)
      .then((fetched) => {
        for (const slug of fetched) slugs.add(slug);
        loaded = true;
      })
      .catch(() => {})
      .finally(() => {
        loading = null;
      });
  }
  return loading;
}

export function isSaved(slug: string): boolean {
  return slugs.has(slug);
}

export function markSaved(slug: string): void {
  slugs.add(slug);
}

export function markUnsaved(slug: string): void {
  slugs.delete(slug);
}
