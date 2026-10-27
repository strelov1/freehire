// Turning on the "jobs matching my profile" alert on one channel — shared by the
// Search alerts tab's toggle (ProfileAlertToggle) and the onboarding tour, so the rules
// below live once.
//
// The alert is a saved search marked derived_from_profile (the server's partial unique
// index allows one per user) built from the profile exactly as "Apply my profile" stages
// it, plus a subscription per channel. Both writes go through their stores, not the raw
// API: the Search alerts tab draws its channel chips from the notifications store, and
// writing past it drew a chip "off" over a live subscription.

import { filtersFromProfile, filtersToParams } from '#lib/filters.js';
import { notifications } from '#lib/notifications.svelte.js';
import { savedSearches } from '#lib/savedSearches.svelte.js';
import type { SavedSearch, UserProfile } from '#lib/types.js';

/** The account's profile alert search, if it has one. */
export function profileAlertSearch(): SavedSearch | null {
  return savedSearches.items.find((s) => s.derived_from_profile) ?? null;
}

/** Create the profile search if there is none, then subscribe it on `channel`.
 *
 *  A search this call created is removed again if the subscribe fails, so a half-done
 *  enable leaves nothing the user never asked for; an existing one is the user's and is
 *  never removed. Throws the subscribe's error. */
export async function enableProfileAlert(profile: UserProfile, channel: string): Promise<void> {
  let search = profileAlertSearch();
  let created: SavedSearch | undefined;
  if (!search) {
    const query = filtersToParams(filtersFromProfile(profile)).toString();
    search = created = await savedSearches.create('My profile', query, true);
  }
  try {
    await notifications.subscribe(search.id, channel);
  } catch (e) {
    if (created) await savedSearches.remove(created.id).catch(() => {});
    throw e;
  }
}
