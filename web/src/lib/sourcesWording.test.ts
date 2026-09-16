import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

// The /sources surface must never describe an aggregator's unmatched postings as
// "exclusive". The dedup pass finding no first-party ATS pair is the absence of evidence —
// its fuzzy and role passes both miss — and the word converts that absence into a claim the
// first time anyone reads it, after which it gets quoted back at us.
//
// The rendered-output check in SourceCatalog.spec.ts covers one card of one component. This
// one covers the SOURCE of every file on the surface, including the page's own prose and
// the group blurbs, which no render of a single card would reach.
// Only the files that produce text a VISITOR reads. types.ts is deliberately absent: its
// doc comment tells the next reader not to render the word, and a guard that cannot tell a
// warning from the thing it warns about teaches people to delete the warning.
const SURFACE = ['../lib/components/SourceCatalog.svelte', '../routes/sources/+page.svelte'];

function read(relative: string): string {
  return readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8');
}

describe('the /sources surface', () => {
  it('never says "exclusive"', () => {
    for (const path of SURFACE) {
      const source = read(path);
      // Anchor per file, so a moved or renamed file fails here rather than passing by
      // being empty — the way a guard quietly stops guarding.
      expect(source.length, `${path} is empty`).toBeGreaterThan(0);
      expect(source.toLowerCase(), `${path} says "exclusive"`).not.toContain('exclusive');
    }
  });

  it('is actually looking at the files that carry the wording', () => {
    // If this stops holding, the list above has drifted from the surface and the guard
    // above is checking files that no longer say anything.
    expect(read('../lib/components/SourceCatalog.svelte')).toContain('not matched to one');
    expect(read('../routes/sources/+page.svelte')).toContain('applicant tracking system');
  });
});
