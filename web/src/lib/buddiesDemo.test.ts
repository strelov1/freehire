import { describe, expect, it } from 'vitest';
import { DEMO_ITEMS, DEMO_POSTS } from './buddiesDemo';
import { buddyHeading } from './buddyPresent';

// A handle is word, word, number. The dashes sit in classes only so the token linter
// does not read them as Tailwind arbitrary values.
const HANDLE = /^[a-z]+[-][a-z]+[-]\d{2,4}$/;

// The landing must never show what the real feed hides: no rejection, no withdrawal,
// no accepted offer. And a demo handle must look like a real one.
describe('buddies demo copy', () => {
  it('uses only event types the real feed shows', () => {
    for (const e of DEMO_ITEMS) expect(['applied', 'interview', 'offer']).toContain(e.type);
  });

  it('uses handles shaped like the real ones', () => {
    for (const e of DEMO_ITEMS) expect(e.handle).toMatch(HANDLE);
  });

  it('gives every member a summary the real dictionaries resolve', () => {
    for (const e of DEMO_ITEMS) {
      expect(buddyHeading(e.card_summary)).not.toBe('Member');
      expect(e.card_summary.country).toMatch(/^[a-z]{2}$/);
    }
  });

  it('reads as one interview post and one folded day of applications', () => {
    expect(DEMO_POSTS.map((p) => p.kind)).toEqual(['event', 'applied']);
  });
});
