// Keep the profile-derived saved search (the "notify me about jobs matching my
// profile" toggle — see ProfileAlertToggle) in step with a changed role/skills/
// location — otherwise it would keep alerting on the profile as it stood when
// first enabled. Best-effort: a failure here never blocks or rolls back whichever
// profile write triggered it, it just leaves the alert stale until the next
// successful save.
//
// Lives in `#lib`, not under a route, because every autosaving surface that
// writes `profileStore` needs to call this — inside `/my/profile` (Role,
// Location, Skills) and outside it (JobMatch's claim/avoid/undo, ProfileForm's
// CV-merge) alike. A route file exporting to `#lib` components would invert the
// only import direction this codebase otherwise uses.

import { filtersFromProfile, filtersToParams } from '#lib/filters.js';
import { profileStore } from '#lib/profile.svelte.js';
import { savedSearches } from '#lib/savedSearches.svelte.js';
import { serialQueue } from '#lib/serialQueue.js';

// Module-level, not per-call: the hazard is two DIFFERENT callers (say, a Skills
// toggle and a JobMatch claim) landing their `savedSearches.update` PUTs out of
// order, so every caller needs to share the same queue, the way `profileStore`'s
// own `#queue` serializes every writer to the profile row itself.
const queue = serialQueue();

async function sync(): Promise<void> {
  if (!profileStore.profile) return;
  await savedSearches.ensureLoaded();
  const p = profileStore.profile;
  if (!p) return;
  const existing = savedSearches.items.find((s) => s.derived_from_profile);
  if (!existing) return;
  try {
    const query = filtersToParams(filtersFromProfile(p)).toString();
    if (!query) {
      // Clearing the last preference must not turn a profile alert into a feed
      // covering every job. Removing it also removes its channel subscriptions.
      await savedSearches.remove(existing.id);
      return;
    }
    await savedSearches.update(existing.id, { query });
  } catch {
    // best-effort — see module doc comment.
  }
}

export function syncProfileAlert(): Promise<void> {
  return queue(sync);
}
