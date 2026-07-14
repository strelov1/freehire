// Builds the HTML for the static brand Open Graph card (light, 1200×630): a
// prominent freehire lockup, the product headline, and a stat-strip of catalogue
// scale. Pure and synchronous — rendered once by scripts/gen-og.mjs into the
// committed web/static/og.png, which is the default og:image for every page
// without its own preview.
//
// Shared brand primitives (mark, escaping) come from ./shared. satori constraint:
// layout is flexbox only, and any element with more than one child declares
// `display:flex`.

import { MARK_DATA_URI, OG_HEIGHT, OG_WIDTH, esc } from './shared';

const HEADLINE = 'The open-source search engine for every job';

export type BrandStat = { value: string; label: string };

function statMarkup(stat: BrandStat): string {
  return (
    `<div style="display:flex;flex-direction:column">` +
    `<div style="display:flex;font-size:52px;font-weight:700;letter-spacing:-0.02em">${esc(stat.value)}</div>` +
    `<div style="display:flex;font-size:24px;color:#a3a3a3">${esc(stat.label)}</div>` +
    `</div>`
  );
}

/** Builds the brand card HTML from the baked-in `stats` figures. */
export function buildBrandCard(opts: { stats: BrandStat[] }): string {
  const statStrip = opts.stats.map(statMarkup).join('');

  return `
<div style="display:flex;flex-direction:column;justify-content:space-between;width:${OG_WIDTH}px;height:${OG_HEIGHT}px;padding:64px 72px;background:#ffffff;color:#0a0a0a;font-family:Inter">
  <div style="display:flex;align-items:center;gap:18px">
    <img src="${MARK_DATA_URI}" style="width:52px;height:52px" />
    <div style="display:flex;font-size:44px;font-weight:700;letter-spacing:-0.03em">freehire</div>
  </div>
  <div style="display:flex;max-width:940px;font-size:64px;font-weight:700;letter-spacing:-0.03em;line-height:1.05">${esc(HEADLINE)}</div>
  <div style="display:flex;align-items:flex-end;justify-content:space-between">
    <div style="display:flex;gap:64px">${statStrip}</div>
    <div style="display:flex;font-size:22px;color:#a3a3a3">freehire.dev</div>
  </div>
</div>`.trim();
}
