// Builds the HTML for one contributor's Open Graph card (light, 1200×630). Pure and
// synchronous — a contributor entry plus a resolved avatar in, an HTML string for
// satori out — so it is exercised directly by unit tests and the render smoke test.
//
// This card is the whole point of the /contributors pages: a link someone posts about
// their own work has to show their face and their figures, not the site's generic
// preview. So the avatar is the hero and the counts are the headline.
//
// Shared brand primitives (mark, escaping, tile, footer) come from ./shared so the
// job/company/contributor cards cannot drift. satori constraint: layout is flexbox
// only, and any element with more than one child declares `display:flex`.

import { contributionSummary } from '$lib/contributors';
import type { ContributorEntry } from '$lib/contributors';
import { OG_HEIGHT, OG_WIDTH, brandFooter, esc, logoBox } from './shared';

const AVATAR_SIZE = 160;

/** "July 2026" — when this person first showed up. */
function firstSeen(iso: string): string {
  return new Date(iso).toLocaleDateString('en', { month: 'long', year: 'numeric', timeZone: 'UTC' });
}

/** Builds the card HTML for `entry`. `avatar` is a data-URI or null — null renders
 *  the shared monogram tile, so an unreachable avatar degrades instead of failing. */
export function buildContributorCard(
  entry: ContributorEntry,
  opts: { avatar: string | null },
): string {
  const role = entry.role === 'maintainer' ? 'Maintains freehire' : 'Contributes to freehire';

  return `
<div style="display:flex;flex-direction:column;justify-content:space-between;width:${OG_WIDTH}px;height:${OG_HEIGHT}px;padding:64px 72px;background:#ffffff;color:#0a0a0a;font-family:Inter">
  <div style="display:flex;align-items:center;gap:32px">
    ${logoBox(opts.avatar, entry.login, AVATAR_SIZE, 'circle')}
    <div style="display:flex;flex-direction:column;gap:10px">
      <div style="display:flex;font-size:60px;font-weight:700;letter-spacing:-0.03em;overflow:hidden">${esc(entry.login)}</div>
      <div style="display:flex;font-size:28px;color:#525252">${esc(role)} since ${esc(firstSeen(entry.firstContributionAt))}</div>
    </div>
  </div>
  <div style="display:flex;font-size:44px;font-weight:700;letter-spacing:-0.02em">${esc(contributionSummary(entry))}</div>
  ${brandFooter()}
</div>`.trim();
}
