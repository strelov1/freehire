import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// The root layout decides which of three footer states a route gets: none (the
// full-bleed account routes, /onboarding, /signin), compact (the rest of /my/*),
// or full (everywhere else). Pins that `hideFooter` reuses `isFullBleedRoute`
// (shellLayout.ts) rather than re-listing /my/assistant/* and /tailor/* inline a
// second time, and that Footer receives `compact` only where that's the intent.
const SOURCE = readFileSync(join(import.meta.dirname, '+layout.svelte'), 'utf8');

describe('root layout footer state', () => {
  it('derives hideFooter from isFullBleedRoute plus onboarding/signin, not a re-listed /my or /tailor/ prefix', () => {
    expect(SOURCE).toContain("import { isFullBleedRoute } from '#lib/shellLayout.js'");
    const hideFooterAt = SOURCE.indexOf('const hideFooter');
    const compactFooterAt = SOURCE.indexOf('const compactFooter', hideFooterAt);
    expect(compactFooterAt, 'expected compactFooter to be declared after hideFooter').toBeGreaterThan(hideFooterAt);
    const hideFooterBlock = SOURCE.slice(hideFooterAt, compactFooterAt);
    expect(hideFooterBlock).toContain('isFullBleedRoute(page.url.pathname)');
    expect(hideFooterBlock).not.toContain("startsWith('/my/')");
    expect(hideFooterBlock).not.toContain("startsWith('/tailor/')");
  });

  it('scopes compactFooter to the account routes that still get a footer at all', () => {
    expect(SOURCE).toMatch(/const isAccountRoute = \$derived\(\s*page\.url\.pathname === '\/my' \|\| page\.url\.pathname\.startsWith\('\/my\/'\)/);
    expect(SOURCE).toContain('const compactFooter = $derived(isAccountRoute && !hideFooter)');
  });

  it('passes compactFooter to Footer as its compact prop', () => {
    expect(SOURCE).toContain('<Footer compact={compactFooter} />');
  });
});
