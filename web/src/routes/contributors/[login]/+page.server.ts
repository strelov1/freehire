import { error, redirect } from '@sveltejs/kit';
import { findContributor } from '$lib/contributors';
import type { ContributorsSnapshot } from '$lib/contributors';
import snapshot from '$lib/data/contributors.json';
import type { PageServerLoad } from './$types';

// One person's page. Resolved through $lib/contributors rather than against the raw
// snapshot, so a bot — or an entry with nothing to show — 404s here exactly as it is
// absent from the showcase, instead of getting a page and a shareable card nothing
// links to.
export const load: PageServerLoad = ({ params }) => {
  const contributor = findContributor(snapshot as ContributorsSnapshot, params.login);
  if (!contributor) error(404, 'Contributor not found');

  // The lookup is case-insensitive because GitHub logins are, but the page has one
  // canonical URL — otherwise every casing is a separate address for the same person,
  // which splits both the search index and whatever a share link accumulates.
  if (contributor.login !== params.login) redirect(308, `/contributors/${contributor.login}`);

  return { contributor };
};
