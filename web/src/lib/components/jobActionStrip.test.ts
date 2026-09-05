import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// The job page's tab row holds two things: the content TabStrip on the left, and the quiet
// action strip on the right. The strip is `shrink-0` and the TabStrip is the half that
// yields, so every control added to the strip comes out of the tab labels. It reached six
// and the tabs were squeezed to a scrolling sliver — "Applications" read as "Applicat…",
// and because TabStrip handles its own overflow correctly (it scrolls and fades rather
// than wrapping) nothing threw, nothing logged, and no test failed.
//
// This is a SOURCE-TEXT AUDIT, and deliberately not a mounted-component test. Two reasons,
// and the second is the load-bearing one:
//
//  1. `web/` has no component-test infrastructure at all — vitest.config.ts runs in plain
//     Node with no Svelte plugin and no DOM (see its own comment, and paginated.svelte.ts's
//     test, which cannot instantiate a `$state` class for the same reason).
//  2. Even with jsdom it would not catch this. "The labels stay readable" is a property of
//     LAYOUT — flexbox, the container's width, real font metrics — and jsdom has no layout
//     engine; `getBoundingClientRect` returns zeros. A mounted test would assert the
//     elements exist, which they always did. Catching the real thing needs a browser with
//     real rendering, which this repo does not run against a PR (pr-smoke is k6 over HTTP;
//     lighthouse-watchdog is Lighthouse against production).
//
// So this guards the CAUSE rather than the symptom: which controls compose the strip. That
// is a genuine proxy and worth naming as one — it will not notice a fifth quiet button, or
// a label long enough to starve the tabs on its own. What it does notice is the specific
// regression that produced the bug: a call to action landing back in the strip. Those are
// the widest controls on the row and the reason it overflowed.
const JOB_VIEW = readFileSync(join(import.meta.dirname, 'JobView.svelte'), 'utf8');

/** The body of a `{#snippet name(...)}` … `{/snippet}` block. Snippets do not nest in this
 *  file, so the first closing tag is the matching one. */
function snippetBody(name: string): string {
  const open = JOB_VIEW.indexOf(`{#snippet ${name}(`);
  if (open === -1) return '';
  const close = JOB_VIEW.indexOf('{/snippet}', open);
  return close === -1 ? '' : JOB_VIEW.slice(open, close);
}

const CTA_SNIPPETS = ['applyCta', 'autoApplyCta'] as const;
const QUIET_CONTROLS = ['reportButton', 'saveButton', 'AddToListButton'] as const;

describe('the job page action strip', () => {
  const strip = snippetBody('actionStrip');
  const ctaGroup = snippetBody('ctaGroup');

  // Guards the audit itself: renaming either snippet would otherwise leave every
  // assertion below passing over an empty string, which is the failure mode of every
  // test that greps.
  it('finds the two snippets it audits', () => {
    expect(strip, 'actionStrip snippet not found in JobView.svelte').not.toBe('');
    expect(ctaGroup, 'ctaGroup snippet not found in JobView.svelte').not.toBe('');
  });

  it.each(CTA_SNIPPETS)('does not render %s', (cta) => {
    expect(strip).not.toContain(`@render ${cta}(`);
  });

  // The other half of the same rule: the CTAs did not simply vanish, they have a home.
  // Without this, deleting them outright would satisfy the assertions above.
  it.each(CTA_SNIPPETS)('leaves %s to the CTA group beside the title', (cta) => {
    expect(ctaGroup).toContain(`@render ${cta}(`);
  });

  it.each(QUIET_CONTROLS)('still carries %s', (control) => {
    expect(strip).toContain(control);
  });

  // The phone-only anchor is the one apply link that is NOT a `Button`: below `lg` the
  // sticky bar belongs to auto-apply, so the strip carries the way out to the posting.
  // It is quiet strip furniture, which is why it is an `<a>` and why it is allowed here
  // when the `applyCta` button is not.
  it('carries the phone-only link out, hidden from lg up', () => {
    expect(strip).toContain('lg:hidden');
    expect(strip).toContain('cta.external.label');
  });
});
