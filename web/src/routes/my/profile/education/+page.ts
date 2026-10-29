import { redirect } from '@sveltejs/kit';

// Education used to be a profile tab of its own; it is now part of a bigger section, so a
// bookmark or an old link lands on it there.
export function load() {
  redirect(308, '/my/profile/experience#account-education');
}
