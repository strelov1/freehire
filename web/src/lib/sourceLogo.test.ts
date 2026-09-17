import { describe, expect, it } from 'vitest';
import { SOURCE_LABELS, sourceLabel } from './facets';
import { SOURCE_LOGO_DOMAINS, sourceLogoUrl } from './logo';

// The logo proxy resolves a brand from a NAME and, when given one, from an explicit
// ?domain= that wins over its name map. Measured against the live proxy 2026-09-17: of the
// 145 sources carrying 500+ jobs, 14 had no logo at all by name — and all 14 resolve when
// the platform's own domain is supplied. `successfactors` is the one that settles the
// argument: its display name was already correct and the proxy still had nothing, so a
// better name alone was never going to be the fix.
//
// The domain also removes a doubt a 200 cannot: a name the proxy resolves WRONGLY answers
// 200 with somebody else's mark, which is how `g2i` wore a stranger's logo for weeks.
describe('source logos', () => {
  it('asks by the platform\'s own domain when one is curated', () => {
    const url = sourceLogoUrl('SAP SuccessFactors', SOURCE_LOGO_DOMAINS.successfactors);

    expect(url).toContain('SAP%20SuccessFactors');
    expect(url).toContain('domain=successfactors.com');
  });

  it('falls back to the name alone when no domain is curated', () => {
    // The map is an OVERRIDE, not a registry: a source missing from it resolves the way it
    // does today rather than losing its logo.
    const url = sourceLogoUrl('Greenhouse', undefined);

    expect(url).toContain('Greenhouse');
    expect(url).not.toContain('domain=');
  });

  it('has no logo URL without a name', () => {
    expect(sourceLogoUrl('', 'greenhouse.io')).toBeNull();
  });

  it('curates bare hosts, never URLs', () => {
    // A scheme or a path here produces a silently broken logo request — the proxy takes a
    // host. Cheap to assert, invisible to review.
    for (const [key, domain] of Object.entries(SOURCE_LOGO_DOMAINS)) {
      expect(domain, `${key}`).toMatch(/^[a-z0-9.-]+\.[a-z]{2,}$/);
      expect(domain, `${key} must not carry a scheme or path`).not.toMatch(/[/:?]/);
    }
  });

  it('gives every curated source a curated display name too', () => {
    // A curated domain means somebody looked the platform up; the name is decided in the
    // same sitting, so it must be an explicit entry rather than a title-cased slug.
    //
    // The test asks for MEMBERSHIP, not for a name that differs from title case: "Freshteam"
    // and "Sber" are the vendors' own spellings and happen to coincide with the fallback.
    // Asserting difference would have demanded a wrong name from two of fourteen.
    for (const key of Object.keys(SOURCE_LOGO_DOMAINS)) {
      expect(SOURCE_LABELS[key], `${key} has a curated domain but no curated name`).toBeTruthy();
      expect(sourceLabel(key)).toBe(SOURCE_LABELS[key]);
    }
  });
});
