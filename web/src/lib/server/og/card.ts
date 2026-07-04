// Builds the HTML for a job's Open Graph card (direction "A — Editorial", light,
// 1200×630). Pure and synchronous: given a job and a resolved logo, it returns an
// HTML string for satori — no I/O, no framework coupling — so it stays cheap to
// reason about and is exercised directly by the render smoke test.
//
// All display strings are reused from $lib/enrichment (the same labels/salary the
// site shows), and every interpolated value is HTML-escaped before it reaches the
// markup. Shared brand primitives (mark, escaping, logo tile, chips, footer) live
// in ./shared so the job/company/brand cards cannot drift.
//
// satori constraint: layout is flexbox only (no CSS grid), and any element with
// more than one child declares `display: flex`.

import type { Job } from '$lib/generated/contracts';
import { formatSalary, seniorityLabel, workArrangement } from '$lib/enrichment';
import { OG_HEIGHT, OG_WIDTH, brandFooter, chipMarkup, esc, logoBox, type Chip } from './shared';

const SKILL_LIMIT = 3;
const LOGO_SIZE = 96;

/** Title font size shrinks as the title grows; a 3-line clamp catches the rest. */
function titleFontSize(title: string): number {
  const n = title.length;
  if (n <= 24) return 64;
  if (n <= 40) return 52;
  if (n <= 64) return 42;
  if (n <= 90) return 34;
  return 30;
}

/** The facet chips, in fixed priority order, with absent facets omitted. */
function chips(job: Job): Chip[] {
  const out: Chip[] = [];
  const wm = workArrangement(job);
  if (wm) out.push({ text: wm });
  const sen = seniorityLabel(job.enrichment);
  if (sen) out.push({ text: sen });
  const salary = formatSalary(job.enrichment);
  if (salary) out.push({ text: salary });

  const skills = job.skills ?? [];
  for (const s of skills.slice(0, SKILL_LIMIT)) out.push({ text: s });
  const extra = skills.length - SKILL_LIMIT;
  if (extra > 0) out.push({ text: `+${extra}`, muted: true });

  return out;
}

/** Builds the card HTML for `job`, with `logo` as a data-URI or null (monogram). */
export function buildCard(job: Job, opts: { logo: string | null }): string {
  const titleSize = titleFontSize(job.title);
  const chipRow = chips(job)
    .map(chipMarkup)
    .join('');

  return `
<div style="display:flex;flex-direction:column;justify-content:space-between;width:${OG_WIDTH}px;height:${OG_HEIGHT}px;padding:64px 72px;background:#ffffff;color:#0a0a0a;font-family:Inter">
  <div style="display:flex;align-items:center;gap:20px">
    ${logoBox(opts.logo, job.company, LOGO_SIZE)}
    <div style="display:flex;font-size:28px;font-weight:600;color:#404040;overflow:hidden">${esc(job.company)}</div>
  </div>
  <div style="display:flex;font-size:${titleSize}px;font-weight:700;letter-spacing:-0.03em;line-height:1.05;overflow:hidden">${esc(job.title)}</div>
  <div style="display:flex;flex-wrap:wrap;gap:12px">${chipRow}</div>
  ${brandFooter()}
</div>`.trim();
}
