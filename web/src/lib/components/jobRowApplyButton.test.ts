import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, the same choice jobActionStrip.test.ts and aiInterviewBadge.test.ts
// make and for the same reason: `web/` has no component-test infrastructure (no Svelte
// plugin, no DOM — see vitest.config.ts), so a mounted test cannot exist here at all. This
// pins the presence rules from freehire#2755 rather than the render output: whether the
// direct-apply shortcut appears (has a destination / has none) and where (full action row /
// compact, which has none).
const ROW = readFileSync(join(import.meta.dirname, 'JobRow.svelte'), 'utf8');

// The compact card's control inventory is a separate `{#if compact} … {:else} … {/if}`
// branch (see the component's own comment on `compact`): splitting on that boundary is what
// lets the assertions below tell "renders in the action row" apart from "renders regardless
// of compact", which a plain `ROW.includes(...)` cannot. The file has other `{#if}/{:else}`
// blocks earlier, so the search for THIS `{:else}` must start at `{#if compact}` itself —
// and if that marker is ever missing, `indexOf` must not be allowed to fall back to search
// from the top of the file and silently pair with one of those earlier, unrelated `{:else}`
// markers, slicing out an empty string that would make every assertion below vacuous. Each
// step below returns '' as soon as a marker is missing, and `'finds the branches it audits'`
// is what turns that into a failure instead of a silent pass.
const compactIfIndex = ROW.indexOf('{#if compact}');
const compactElseIndex = compactIfIndex === -1 ? -1 : ROW.indexOf('{:else}', compactIfIndex);
const compactBranch = compactElseIndex === -1 ? '' : ROW.slice(compactIfIndex, compactElseIndex);
const actionRow = compactElseIndex === -1 ? '' : ROW.slice(compactElseIndex, ROW.lastIndexOf('{/if}'));
// The button's own `{#if applyJob} … {/if}` block, isolated within the action row so the
// hover-reveal and link-hygiene assertions can't pass by matching Hide's copies instead.
const applyIfIndex = actionRow.indexOf('{#if applyJob}');
const applyBlock = applyIfIndex === -1 ? '' : actionRow.slice(applyIfIndex, actionRow.indexOf('{/if}', applyIfIndex));

describe('JobRow direct-apply button', () => {
  it('finds the branches it audits', () => {
    expect(compactBranch, 'compact branch not found in JobRow.svelte').not.toBe('');
    expect(actionRow, 'action row not found in JobRow.svelte').not.toBe('');
    expect(applyBlock, 'applyJob block not found in JobRow.svelte').not.toBe('');
  });

  it('is gated on a job that actually carries an outbound url', () => {
    expect(ROW).toContain("const applyJob = $derived('url' in job ? job : null);");
    expect(applyBlock).not.toBe('');
  });

  it('never renders on the compact card', () => {
    expect(compactBranch).not.toContain('applyJob');
  });

  it('is outline, never the brand-green primary', () => {
    expect(applyBlock).toContain('variant="outline"');
  });

  it('carries the same link hygiene as the job page apply link', () => {
    expect(applyBlock).toContain('target="_blank"');
    expect(applyBlock).toContain('rel="nofollow noopener noreferrer"');
  });

  it('fires the same apply-intent event the job page fires', () => {
    expect(ROW).toContain("track('job_apply', { slug: applyJob.public_slug, source: applyJob.source })");
  });

  it('is a sibling of the card link, inside the action row', () => {
    // The action row itself sits after the closing `</a>` of the card link.
    expect(ROW.indexOf('</a>')).toBeLessThan(ROW.indexOf('{#if applyJob}'));
  });

  it('hover-reveals like Hide, and stays visible on touch', () => {
    for (const cls of [
      'opacity-0',
      'focus-visible:opacity-100',
      'group-hover:opacity-100',
      'pointer-coarse:opacity-100',
    ]) {
      expect(applyBlock).toContain(cls);
    }
  });
});
