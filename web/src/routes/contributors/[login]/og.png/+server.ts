import { error } from '@sveltejs/kit';
import { findContributor } from '$lib/contributors';
import type { ContributorsSnapshot } from '$lib/contributors';
import snapshot from '$lib/data/contributors.json';
import { resolveAvatar } from '$lib/server/og/avatar';
import { buildContributorCard } from '$lib/server/og/contributor';
import { loadOgFonts } from '$lib/server/og/fonts';
import { renderMarkupPng } from '$lib/server/og/render';
import type { RequestHandler } from './$types';

// Renders one contributor's 1200×630 Open Graph preview on demand. This is the card a
// contributor's own shared link unfurls into, which is what makes the profile page worth
// posting at all — without it every contributor's link previews as the generic site
// image and none of them looks like it is about a person.
//
// Resolved by the same lookup the page uses, so a login the showcase does not list —
// absent, a bot, nothing to show — 404s here with no image rather than getting a
// shareable card nothing links to. The avatar fetch degrades to a monogram on any
// failure, so it never blocks the response. Cached for an hour with a day of
// stale-while-revalidate, matching the site's other cards: the snapshot behind it
// changes at most once a day.
export const GET: RequestHandler = async ({ params }) => {
  const contributor = findContributor(snapshot as ContributorsSnapshot, params.login);
  if (!contributor) error(404, 'Contributor not found');

  const [fonts, avatar] = await Promise.all([
    loadOgFonts(),
    resolveAvatar(contributor.avatarUrl),
  ]);

  const png = await renderMarkupPng(buildContributorCard(contributor, { avatar }), fonts);

  return new Response(png, {
    headers: {
      'content-type': 'image/png',
      'cache-control': 'public, max-age=3600, stale-while-revalidate=86400',
    },
  });
};
