import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT (web/ has no component-test infrastructure; see
// jobRowApplyButton.test.ts). The referral form shares a CV tailored for the request's
// vacancy or company, or the uploaded one — never a pick from every CV the seeker owns,
// which once offered CVs tailored for other companies.
const MODAL = readFileSync(join(import.meta.dirname, 'RequestReferralModal.svelte'), 'utf8');

describe('RequestReferralModal CV choice', () => {
  it('asks the server what fits instead of listing every CV', () => {
    expect(MODAL).not.toContain('listCvs');
    expect(MODAL).toContain('api.getReferralCvOptions(companySlug, jobSlug)');
  });

  it('attaches the fitting tailored CV by itself', () => {
    expect(MODAL).toContain("cvKind = 'built';\n        cvId = first.id;");
  });

  it('offers tailoring only where there is a vacancy to tailor for', () => {
    expect(MODAL).toContain('{#if onTailor}');
  });
});
