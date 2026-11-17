import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, not a mounted-component test — web/ has no component-test
// infrastructure at all (see jobActionStrip.test.ts's own comment: no Svelte
// plugin, no DOM in vitest.config.ts).
//
// The root layout decides which of three footer states a route gets: none (the
// full-bleed account routes, /onboarding, /signin), compact (the rest of /my/* and /buddies),
// or full (everywhere else). Pins that `hideFooter` reuses `isFullBleedRoute`
// (shellLayout.ts) rather than re-listing /my/assistant/* and /tailor/* inline a
// second time, and that Footer receives `compact` only where that's the intent.
const SOURCE = readFileSync(join(import.meta.dirname, '+layout.svelte'), 'utf8');

describe('root layout footer state', () => {
  it('derives hideFooter from isFullBleedRoute plus onboarding/signin, not a re-listed /my or /tailor/ prefix', () => {
    expect(SOURCE).toContain("import { isAccountArea, isFullBleedRoute } from '#lib/shellLayout.js'");
    const hideFooterAt = SOURCE.indexOf('const hideFooter');
    const compactFooterAt = SOURCE.indexOf('const compactFooter', hideFooterAt);
    expect(compactFooterAt, 'expected compactFooter to be declared after hideFooter').toBeGreaterThan(hideFooterAt);
    const hideFooterBlock = SOURCE.slice(hideFooterAt, compactFooterAt);
    expect(hideFooterBlock).toContain('isFullBleedRoute(page.url.pathname)');
    expect(hideFooterBlock).not.toContain("startsWith('/my/')");
    expect(hideFooterBlock).not.toContain("startsWith('/tailor/')");
  });

  it('scopes compactFooter to the account routes that still get a footer at all', () => {
    // isAccountArea: /my and the buddies space, the one shared answer to "the account area".
    expect(SOURCE).toMatch(/const isAccountRoute = \$derived\(isAccountArea\(page\.url\.pathname\)\)/);
    expect(SOURCE).toContain('const compactFooter = $derived(isAccountRoute && !hideFooter)');
  });

  it('passes compactFooter to Footer as its compact prop', () => {
    expect(SOURCE).toContain('<Footer compact={compactFooter} />');
  });
});

// The onboarding redirect must not fire on /signin. Signing in there flips the session and
// the sign-in page then navigates to its own returnTo; a redirect fired in between raced
// that navigation and lost, so the tour opened for an instant — long enough to record
// itself as shown — and the visitor never saw it (2026-10-27, 18 accounts). The sign-in
// page lands the visitor first; the gate runs on the page it lands on.
//
// The flow on /onboarding is open to everyone (cv-first-onboarding): an account that has
// not seen it is routed there once, and nothing routes anybody AWAY from it — neither a
// signed-out visitor (who can run it to the results) nor an account that completed it.
describe('root layout onboarding gate', () => {
  const gateAt = SOURCE.indexOf('const needsOnboarding');
  const gate = SOURCE.slice(gateAt, SOURCE.indexOf('});', gateAt));

  // The rules themselves (the /signin and /onboarding guards, once per visit, a guest's skip)
  // are behaviour of routeAccount, exercised in onboardingGate.spec.ts; the layout only has to
  // hand it a signed-in account's path and need.
  it('hands only a signed-in account, with its need, to the onboarding gate', () => {
    expect(SOURCE).toContain('const needsOnboarding = $derived(currentUser()?.onboarding_completed_at == null)');
    expect(gate).toContain('if (isAuthenticated()) {');
    expect(gate).toContain('routeAccount(page.url.pathname, page.url.search, needsOnboarding');
    expect(gate).not.toContain("path !== '/signin'"); // not re-listed beside the rule that owns it
  });

  it('never bounces anybody away from /onboarding', () => {
    // The reverse bounce read the completion marker while ON the page; any read of the
    // pathname as `=== '/onboarding'` outside the footer rule would be one again.
    const outsideFooter = SOURCE.replace("page.url.pathname === '/onboarding' ||", '');
    expect(outsideFooter).not.toContain("=== '/onboarding'");
    expect(SOURCE).not.toContain('onboarding_completed_at != null');
  });
});

describe('root layout sign-out reset', () => {
  it('clears the guest skip mark only on a signed-in to signed-out transition', () => {
    const at = SOURCE.indexOf('let wasAuthenticated');
    const block = SOURCE.slice(at, SOURCE.indexOf('});', at));
    expect(block).toContain('if (wasAuthenticated) onboardingGate.reset();');
    expect(block).not.toMatch(/if \(!authenticated\) \{\s*resetUserStores\(\);\s*onboardingGate\.reset\(\)/);
  });
});
