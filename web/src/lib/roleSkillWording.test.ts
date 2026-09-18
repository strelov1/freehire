import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

// The role leaf page's wording is NORMATIVE, not editorial. jobs.skills tags a skill named
// anywhere in a posting — a nice-to-have list, a stack blurb — so every share it renders is
// the ceiling on what the role requires, not the bar. The spec
// (openspec/specs/market-insights) says every surface rendering it SHALL say "mentioned in"
// and never "required by", and a rule with no test is a rule until the next edit.
//
// Guarded the same way `ats_unmatched` is: over the SOURCE of every file on the surface,
// not just over one rendered string, since the wording can drift into a heading, a tooltip
// or the intro sentence just as easily.
const SURFACE = 'src/routes/insights/roles/[category]/[seniority]';

/** What a VISITOR reads, which is what the rule is about.
 *
 *  Comments go first: the forbidden phrasing is quoted in this change's own comments —
 *  that is where the REASON lives — and a guard that could not tell an explanation from
 *  a claim would push the explanation out of the code.
 *
 *  Then markup, then whitespace. Without those two a page could render
 *  `required <strong>by</strong>`, which a visitor reads as the forbidden phrase and a
 *  tag-blind matcher walks straight past — the guard would be green about the one thing
 *  it exists to catch. `checks its own tag-split blind spot` below holds that closed. */
function visibleText(text: string): string {
  return text
    .replace(/<!--[\s\S]*?-->/g, ' ')
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .replace(/(^|\s)\/\/.*$/gm, ' ')
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ');
}

/** The forbidden phrasings, as one place both the guard and its own test read. */
const FORBIDDEN = /required by|requires these|required skills/i;
const MARKET_CLAIM = /the market for|market rate|across the market/i;

function sources(dir: string): { path: string; text: string }[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = join(dir, e.name);
    return e.isDirectory()
      ? sources(p)
      : [{ path: p, text: visibleText(readFileSync(p, 'utf8')) }];
  });
}

describe('role leaf page wording', () => {
  const files = sources(SURFACE);

  it('has files to check (so a moved route fails loudly rather than passing empty)', () => {
    expect(files.length).toBeGreaterThan(0);
  });

  it('says "mentioned in" somewhere on the surface', () => {
    const said = files.some((f) => /mentioned in/i.test(f.text));
    expect(said, `no file under ${SURFACE} says "mentioned in"`).toBe(true);
  });

  it('never claims a skill is required by the role', () => {
    for (const f of files) {
      // "required" alone is too broad — it appears in ordinary prose. What the spec
      // forbids is presenting the share as a requirement.
      expect(f.text, `${f.path} presents the share as a requirement`).not.toMatch(FORBIDDEN);
    }
  });

  it('never claims the figures describe the role\'s market', () => {
    // Only 39% of open technical postings state a seniority, so this slice is the
    // postings that SAY a level, not the market for it.
    for (const f of files) {
      expect(f.text, `${f.path} claims to describe the market`).not.toMatch(MARKET_CLAIM);
    }
  });

  it('checks its own tag-split blind spot', () => {
    // A guard that only ever passes proves nothing. These are the shapes a page could
    // ship that a visitor reads as the forbidden claim.
    expect(visibleText('<p>required <strong>by</strong> this role</p>')).toMatch(FORBIDDEN);
    expect(visibleText('<span>across\n  the\tmarket</span>')).toMatch(MARKET_CLAIM);
    // And the reason, written in a comment, must stay allowed.
    expect(visibleText('<!-- never say "required by" here -->')).not.toMatch(FORBIDDEN);
  });
});
