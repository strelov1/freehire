import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

import { ApiError, createApi, RESUME_MAX_MB } from '$lib/api';
import { marketLine, roleDisplay } from './roastView';

// `web/` has no component-mount test infrastructure at all: vitest.config.ts runs in
// plain Node with no Svelte plugin and no DOM (see paginated.svelte.test.ts's own note,
// and jobActionStrip.test.ts, which audits JobView.svelte's source text for the same
// reason). This file follows both of those, not the brief's illustrative
// `render(Page, ...)` snippet, which this repo's harness cannot run:
//
//  1. The client wiring (`api.roastCv`) is real logic and gets a real unit test against
//     `createApi`, the same way api.test.ts covers every other endpoint.
//  2. The pure response→view derivations (`roleDisplay`/`marketLine`) live in
//     ./roastView.ts specifically so they can be unit tested without a component, the
//     same reason `paginated.svelte.ts` exists.
//  3. What the markup itself does with those derivations — which testid renders in which
//     branch, and that the drop zone is not gated behind a session check — is a
//     source-text audit of +page.svelte, the same technique jobActionStrip.test.ts uses
//     for a property that a mount would not catch either.

// --- 1. api.roastCv: the client wiring -----------------------------------------------

function fakeResponse(body: unknown): Response {
  return new Response(JSON.stringify({ data: body }), { status: 200 });
}

const SAMPLE_REPORT = {
  overall: 42,
  potential: 60,
  categories: [],
  strong_keywords: [],
  recommended_keywords: [],
  reviewed: false,
};

describe('api.roastCv', () => {
  it('sends pasted text as JSON to /api/v1/cv/roast, no category by default', async () => {
    const seen: { url?: string; init?: RequestInit } = {};
    const fetcher = ((url: string, init?: RequestInit) => {
      seen.url = url;
      seen.init = init;
      return Promise.resolve(fakeResponse({ report: SAMPLE_REPORT, role: '', market_scoped: false, market_available: false }));
    }) as unknown as typeof fetch;

    await createApi(fetcher).roastCv('Some CV text');

    expect(seen.url).toBe('/api/v1/cv/roast');
    expect(seen.init?.method).toBe('POST');
    expect(seen.init?.body).toContain('Some CV text');
    const headers = seen.init?.headers as Record<string, string> | undefined;
    expect(headers?.['Content-Type']).toBe('application/json');
  });

  it('sends a PDF File as multipart form data', async () => {
    const seen: { init?: RequestInit } = {};
    const fetcher = ((_url: string, init?: RequestInit) => {
      seen.init = init;
      return Promise.resolve(fakeResponse({ report: SAMPLE_REPORT, role: 'backend', market_scoped: true, market_available: false }));
    }) as unknown as typeof fetch;
    const file = new File(['%PDF-1.4 fake'], 'cv.pdf', { type: 'application/pdf' });

    await createApi(fetcher).roastCv(file);

    const body = seen.init?.body as FormData | undefined;
    expect(body).toBeInstanceOf(FormData);
    expect(body?.get('file')).toBe(file);
  });

  it('appends ?category= when a role override is given', async () => {
    const urls: string[] = [];
    const fetcher = ((url: string) => {
      urls.push(url);
      return Promise.resolve(fakeResponse({ report: SAMPLE_REPORT, role: 'devops', market_scoped: true, market_available: false }));
    }) as unknown as typeof fetch;

    await createApi(fetcher).roastCv('text', 'devops');

    expect(urls).toEqual(['/api/v1/cv/roast?category=devops']);
  });

  it('refuses an oversize PDF before making a request', async () => {
    let called = false;
    const fetcher = (() => {
      called = true;
      return Promise.resolve(fakeResponse({}));
    }) as unknown as typeof fetch;
    const big = new File([new Uint8Array(RESUME_MAX_MB * 1024 * 1024 + 1)], 'cv.pdf', {
      type: 'application/pdf',
    });

    await expect(createApi(fetcher).roastCv(big)).rejects.toBeInstanceOf(ApiError);
    expect(called).toBe(false);
  });
});

// --- 2. roleDisplay / marketLine: pure response→view derivations ---------------------

describe('roleDisplay', () => {
  it('reads market_scoped, not whether role is empty', () => {
    expect(roleDisplay({ role: 'backend', market_scoped: true })).toEqual({
      scoped: true,
      role: 'backend',
    });
  });

  it('reports unscoped when the dictionary resolved nothing', () => {
    expect(roleDisplay({ role: '', market_scoped: false })).toEqual({ scoped: false, role: '' });
  });
});

describe('marketLine', () => {
  const scopedVerdict = {
    total: 500,
    covered: 300,
    coverage_percent: 60,
    gaps: [{ name: 'kubernetes', new_vacancies: 120, unlock_percent: 24 }],
    skills: [],
    must_have_total: 0,
    must_have_covered: 0,
    stack_match_percent: 0,
    coherence_percent: 0,
    bundles: [],
  };

  it('returns the covered/total/topGap reading when the market is available', () => {
    expect(marketLine({ market_available: true, market: scopedVerdict })).toEqual({
      covered: 300,
      total: 500,
      coveragePercent: 60,
      topGap: { name: 'kubernetes', new_vacancies: 120, unlock_percent: 24 },
    });
  });

  it('returns null — never a zeroed object — when the backend could not answer', () => {
    expect(marketLine({ market_available: false, market: undefined })).toBeNull();
  });

  it('is defensive against market_available true with no market body', () => {
    // Should not happen per the backend contract, but a null derivation is the safe
    // reading if it ever does — never a fabricated zero.
    expect(marketLine({ market_available: true, market: undefined })).toBeNull();
  });

  it('reports no gap (not a fabricated one) when the CV already covers every skill', () => {
    const fullyCovered = { ...scopedVerdict, gaps: [] };
    expect(marketLine({ market_available: true, market: fullyCovered })?.topGap).toBeNull();
  });
});

// --- 3. +page.svelte: a source-text audit of the markup itself -----------------------

const PAGE = readFileSync(join(import.meta.dirname, '+page.svelte'), 'utf8');

// Resolved from the package root (vitest's process.cwd() is `web/` — confirmed by every
// other test's own relative import paths), not from this test file's own directory, so
// relocating this test file cannot silently change which route directory gets inspected.
const ROAST_ROUTE_DIR = join(process.cwd(), 'src/routes/roast');

describe('/roast page markup', () => {
  // A test asserting a property of THIS TEST FILE's own path (e.g. "import.meta.dirname
  // does not contain /routes/my/") cannot fail from any change to the page's actual
  // routing or auth behavior — it is a tautology about where the test happens to live,
  // not a check of the route tree. SvelteKit's directory rules mean the only place a
  // session gate could attach to this route SPECIFICALLY (short of editing the shared
  // root +layout.server.ts, which every public page relies on staying gate-free) is a
  // +layout.server.ts / +layout.ts / +layout.svelte dropped right into this directory —
  // see web/src/routes/my/+layout.server.ts for the exact shape such a gate takes
  // (a `redirect(302, signinUrl(...))` when `parent()`'s `user` is absent). This asserts
  // none of the three exist, so a future PR introducing one here — even by accident —
  // fails this test instead of silently gating an account-free page.
  it('carries no layout of its own — the only place a session gate could attach to this route', () => {
    for (const name of ['+layout.server.ts', '+layout.ts', '+layout.svelte']) {
      expect(
        existsSync(join(ROAST_ROUTE_DIR, name)),
        `${name} must not exist under src/routes/roast — its presence would be a session gate scoped to this route (see web/src/routes/my/+layout.server.ts for the shape one takes)`,
      ).toBe(false);
    }
  });

  it('renders the drop zone unconditionally, not behind an auth check', () => {
    const idx = PAGE.indexOf('data-testid="roast-dropzone"');
    expect(idx).toBeGreaterThan(-1);
    // The nearest preceding template conditional (if any) must not be an isAuthenticated/
    // session gate — the whole point of this page is that no session is required to see it.
    const before = PAGE.slice(0, idx);
    expect(before).not.toContain('isAuthenticated');
  });

  it('names the role and offers a change control together', () => {
    expect(PAGE).toContain('data-testid="roast-role"');
    expect(PAGE).toContain('data-testid="roast-role-change"');
  });

  it('says the reading is unscoped instead of presenting an empty role as a real one', () => {
    expect(PAGE).toContain('data-testid="roast-role-unscoped"');
    // The unscoped branch must be the {:else} of the same condition the scoped branch
    // reads, so the two can never both render.
    const scopedIf = PAGE.indexOf('{#if role?.scoped}');
    const unscopedTestid = PAGE.indexOf('data-testid="roast-role-unscoped"');
    expect(scopedIf).toBeGreaterThan(-1);
    expect(unscopedTestid).toBeGreaterThan(scopedIf);
  });

  it('never falls back to a literal zero when the market is unavailable', () => {
    const idx = PAGE.indexOf('data-testid="roast-market-unavailable"');
    expect(idx).toBeGreaterThan(-1);
    // The unavailable note must come from the `{:else}` of `{#if market}` — i.e. it
    // must not itself read market.covered/market.total with a `?? 0` fallback, which
    // is the exact regression this note exists to prevent.
    const noteLine = PAGE.slice(idx - 40, idx + 200);
    expect(noteLine).not.toContain('?? 0');
    expect(noteLine).not.toContain('market.covered');
  });

  it('calls api.roastCv rather than hand-rolling a fetch', () => {
    expect(PAGE).toContain('api.roastCv(');
    expect(PAGE).not.toMatch(/\bfetch\(/);
  });

  it('ends on a sign-in call to action that opens the register form on the signed-in continuation', () => {
    // Not promptSignIn(): that helper opens the LOGIN form and returns to the anonymous
    // page it was called from, which is wrong for an audience that has never heard of us
    // and has no account to log into. This must open the register form (no `mode`) and
    // land on the page the CTA promises.
    expect(PAGE).toContain('signinUrl({ returnTo: \'/my/profile/cv-readiness\' })');
    expect(PAGE).not.toContain('promptSignIn');
  });
});

// changeRole's body, isolated so these assertions cannot accidentally match roast()'s own
// (near-identical) guard a few lines above it.
const CHANGE_ROLE_START = PAGE.indexOf('async function changeRole');
const CHANGE_ROLE_END = PAGE.indexOf('function onRoleChange', CHANGE_ROLE_START);
const CHANGE_ROLE_BODY = PAGE.slice(CHANGE_ROLE_START, CHANGE_ROLE_END);

describe('changeRole guards', () => {
  it('is found, and is not empty — a prerequisite for every assertion below', () => {
    expect(CHANGE_ROLE_START).toBeGreaterThan(-1);
    expect(CHANGE_ROLE_END).toBeGreaterThan(CHANGE_ROLE_START);
  });

  // A role change in flight must not win a race against a newer upload: without this,
  // dropping a new CV while a role-change request is still pending can have the stale
  // response land last and overwrite the new CV's report with the old CV's re-scored one.
  it('uses the same out-of-order guard as roast(), so a stale response cannot overwrite a newer one', () => {
    expect(CHANGE_ROLE_BODY).toMatch(/const my = \+\+gen;/);
    expect(CHANGE_ROLE_BODY).toMatch(/if \(my !== gen\) return;/);
  });

  // A failed role change must tell the visitor, not fail silently — the budget backing
  // it (cvRoastPerHour) is shared with roast() and can already be spent by the time the
  // picker is used.
  it('tells the visitor when a role change fails, instead of failing silently', () => {
    expect(CHANGE_ROLE_BODY).not.toContain('say nothing further');
    expect(CHANGE_ROLE_BODY).toMatch(/status = 'error';/);
    expect(CHANGE_ROLE_BODY).toMatch(/error = /);
  });

  // The failure path must reset the picker's own displayed value — otherwise a native
  // <select> keeps showing the role the visitor clicked even though the reading on the
  // page still describes the previous one.
  it('resets the picker so it cannot show a role the reading was never scored against', () => {
    expect(CHANGE_ROLE_BODY).toContain('roleChangeResetKey++');
  });
});

describe('the role picker', () => {
  // Svelte only forces a native <select>'s displayed value back onto the DOM when the
  // element itself is torn down and rebuilt — a plain reactive `value={...}` binding is
  // not enough once the browser's own selection has drifted from it. {#key} is how this
  // page does that rebuild, keyed on the counter changeRole's catch bumps.
  it('is wrapped in a {#key} block keyed on roleChangeResetKey', () => {
    const selectIdx = PAGE.indexOf('data-testid="roast-role-change"');
    expect(selectIdx).toBeGreaterThan(-1);
    const before = PAGE.slice(0, selectIdx);
    const keyIdx = before.lastIndexOf('{#key roleChangeResetKey}');
    expect(keyIdx).toBeGreaterThan(-1);
    // No other {/key} between the block's opening and the <select> itself.
    const between = PAGE.slice(keyIdx, selectIdx);
    expect(between).not.toContain('{/key}');
  });
});
