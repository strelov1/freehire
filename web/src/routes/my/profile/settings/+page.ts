import { redirect } from '@sveltejs/kit';

// The profile used to have a Settings section holding the account's timezone and language.
// Both moved where they belong — the timezone into Location, beside where the candidate is,
// and the language to /my/security with the rest of the account — so a bookmark or an old
// link here lands on the timezone, the one of the two that links point at.
export function load() {
  redirect(308, '/my/profile/location#account-timezone');
}
