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
// This is a SOURCE-TEXT AUDIT, and deliberately not a mounted-component test, even though
// `web/` now has one: vitest.config.ts's `components` project renders real `.svelte` files
// via `@testing-library/svelte` + jsdom (`*.spec.ts`, added for the profile-alert-sync fix).
// That project would not catch THIS bug regardless: "the labels stay readable" is a property
// of LAYOUT — flexbox, the container's width, real font metrics — and jsdom has no layout
// engine; `getBoundingClientRect` returns zeros. A mounted test would assert the elements
// exist, which they always did. Catching the real thing needs a browser with real rendering,
// which this repo does not run against a PR (pr-smoke is k6 over HTTP; lighthouse-watchdog is
// Lighthouse against production).
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

/** The pinned header's button row: the one place the page repeats its CTAs verbatim while
 *  the title is scrolled away. Anchored on the icon-only Save only it renders, and cut at
 *  the row's own closing tag — nothing inside it opens a `div`. */
function pinnedHeaderButtons(): string {
  const open = JOB_VIEW.indexOf("{@render saveButton('size-9 rounded-md px-0', true)}");
  if (open === -1) return '';
  const close = JOB_VIEW.indexOf('</div>', open);
  return close === -1 ? '' : JOB_VIEW.slice(open, close);
}

/** The phone's sticky bottom bar, also not a snippet. Anchored on the glass panel's own
 *  class — `pointer-events-none sticky bottom-0` appears nowhere else — and cut at the
 *  panel's closing tag; the `{#if}` inside it opens no element. */
function stickyBar(): string {
  const open = JOB_VIEW.indexOf('pointer-events-none sticky bottom-0');
  if (open === -1) return '';
  const close = JOB_VIEW.indexOf('</div>', open);
  return close === -1 ? '' : JOB_VIEW.slice(open, close);
}

// The same audit, one row up: which controls compose the page's CALL TO ACTION, and how
// loud each is. This is the rule `autoApplyButton.ts` can no longer hold on its own — it
// ranks the two apply controls against each other, while the thing that outranks both is a
// third button it never sees. A `variant` drifting here is exactly the bug jsdom cannot
// catch: every element still renders, nothing throws, and the page simply grows a second
// green button or loses its only one.
//
// Three positions, because a primary CTA that exists at one width and not another is the
// same defect as one that does not exist at all.
describe('the job page call-to-action row', () => {
  const tailorCta = snippetBody('tailorCta');
  const applyCta = snippetBody('applyCta');
  const autoApplyCta = snippetBody('autoApplyCta');
  const ctaGroup = snippetBody('ctaGroup');
  const pinnedHeader = pinnedHeaderButtons();

  it('finds every region it audits', () => {
    expect(tailorCta, 'tailorCta snippet not found in JobView.svelte').not.toBe('');
    expect(ctaGroup, 'ctaGroup snippet not found in JobView.svelte').not.toBe('');
    expect(pinnedHeader, 'pinned header button row not found in JobView.svelte').not.toBe('');
    expect(stickyBar(), 'phone sticky bar not found in JobView.svelte').not.toBe('');
  });

  it.each([
    ['the title row', () => ctaGroup],
    ['the pinned header', () => pinnedHeader],
  ])('renders tailorCta in %s', (_where, region) => {
    expect(region()).toContain('@render tailorCta(');
  });

  // The sticky bar is not a snippet, and `pointer-events-auto` is the class only it passes
  // — the bar's own glass panel is `pointer-events-none`, so every button in it must
  // re-enable them for itself.
  it('renders tailorCta in the phone sticky bar', () => {
    expect(stickyBar()).toContain('@render tailorCta(');
  });

  // The bar holds two buttons at `flex-1`. A third would either shrink all three past
  // legibility or wrap the row and make the bar taller, and it would offer a reader two
  // ways to apply to one posting — the ambiguity `leads` exists to resolve. The `{:else}`
  // is what keeps it to one, so it is the branch, not the count, that is worth pinning.
  it('offers exactly one apply control in the phone sticky bar', () => {
    const bar = stickyBar();
    expect(bar.match(/@render autoApplyCta\(/g) ?? []).toHaveLength(1);
    expect(bar.match(/@render applyCta\(/g) ?? []).toHaveLength(1);
    expect(bar).toContain('{:else}');
  });

  // Ascending rank left to right, so the brand fill lands at the row's end rather than in
  // its middle. Asserted by ORDER rather than by presence: all three could be rendered and
  // still read wrong.
  it.each([
    ['the title row', () => ctaGroup],
    ['the pinned header', () => pinnedHeader],
  ])('renders tailorCta last in %s', (_where, region) => {
    const source = region();
    const order = ['autoApplyCta', 'applyCta', 'tailorCta'].map((s) =>
      source.indexOf(`@render ${s}(`),
    );
    expect(order.every((i) => i !== -1)).toBe(true);
    expect(order).toEqual([...order].sort((a, b) => a - b));
  });

  it('gives the brand fill to tailorCta alone', () => {
    expect(tailorCta).toContain('variant="primary"');
    expect(applyCta).not.toContain('primary');
    expect(autoApplyCta).not.toContain('primary');
  });

  it('renders the apply link as an outline button in every state', () => {
    expect(applyCta).toContain('variant="outline"');
  });

  // `primary` on the CTA plan is gone: it answered "is this brand-filled" and "is auto-apply
  // the offered way to apply" at once, and only the second question survives — as `leads`.
  // A surviving read would be `undefined`, which is falsy, so the phone's bar would quietly
  // drop auto-apply (it has no other home below `lg`) and the quiet strip would drop the
  // origin link with it. Nothing would throw.
  it('reads leads, never primary, off the CTA plan', () => {
    expect(JOB_VIEW).not.toContain('cta.autoApply?.primary');
    expect(JOB_VIEW).not.toContain('external.primary');
    expect(JOB_VIEW).toContain('cta.autoApply?.leads');
  });
});
