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

  it('is gated on an open job that actually carries an outbound url', () => {
    // A non-empty one: a vacancy taking applications on freehire carries `url: ""`. And an
    // open one: a closed posting takes no applications, on any surface that lists it.
    expect(ROW).toContain(
      "const applyJob = $derived('url' in job && job.url && !job.closed_at ? job : null);",
    );
    expect(ROW).toContain("job.direct_apply === true && !job.closed_at");
    expect(applyBlock).not.toBe('');
  });

  it('sends a vacancy taking applications on freehire to its own page, not outside', () => {
    const i = actionRow.indexOf('{#if directApply}');
    const block = i === -1 ? '' : actionRow.slice(i, actionRow.indexOf('{/if}', i));
    expect(block).toContain("href={resolve('/jobs/[slug]', { slug: job.public_slug })}");
    expect(block).not.toContain('target="_blank"');
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

  it('names the job it applies to, and which of the two destinations it is', () => {
    // A feed holds twenty "Apply" buttons; a name without the job is twenty identical
    // entries in a screen reader's control list.
    expect(applyBlock).toContain("`Apply to ${job.title} on the employer's site (opens in a new tab)`");
    // Signed out, the button opens a choice rather than sending the visitor straight to sign-in.
    expect(applyBlock).toContain('`Choose how to apply to ${job.title}`');
    const i = actionRow.indexOf('{#if directApply}');
    const block = i === -1 ? '' : actionRow.slice(i, actionRow.indexOf('{/if}', i));
    expect(block).toContain('aria-label="Apply to {job.title}"');
  });

  it('announces the apply choices through a persistently mounted live region', () => {
    expect(ROW).toContain('<div class="sr-only" aria-live="polite" aria-atomic="true">');
    expect(ROW).toContain(
      'Choose how to apply to {job.title}: sign in, sign up, or continue without signing in.',
    );
  });
});

describe('JobRow card link', () => {
  it('is named by title and company, not by every word on the card', () => {
    expect(ROW).toContain('aria-label={linkName}');
    expect(ROW).toContain("const linkName = $derived(`${job.title} at ${job.company || 'Unknown company'}`);");
  });

  it('dims a viewed card no further than 90%, which keeps muted text above 4.5:1', () => {
    expect(ROW).toContain('class:opacity-90={isViewed}');
    expect(ROW).not.toContain('class:opacity-80');
  });
});
