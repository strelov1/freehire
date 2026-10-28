import { describe, expect, it } from 'vitest';
import { DEMO_FEED, DEMO_INVITE, DEMO_REPLY } from './buddiesDemo';
import { buddyHeading } from './buddyPresent';

// A handle is word, word, number. The dashes sit in classes only so the token linter
// does not read them as Tailwind arbitrary values.
const HANDLE = /^[a-z]+[-][a-z]+[-]\d{2,4}$/;

// The landing must never show what the real feed hides: no rejection, no withdrawal,
// no accepted offer. And a demo handle must look like a real one.
describe('buddies demo copy', () => {
  it('uses only event types the real feed shows', () => {
    for (const e of DEMO_FEED) expect(['applied', 'interview', 'offer']).toContain(e.type);
  });

  it('uses handles shaped like the real ones', () => {
    for (const e of DEMO_FEED) expect(e.handle).toMatch(HANDLE);
    expect(DEMO_INVITE.to).toMatch(HANDLE);
  });

  it('invites someone who is in the feed, within the 500-character limit', () => {
    expect(DEMO_FEED.map((e) => e.handle)).toContain(DEMO_INVITE.to);
    expect(DEMO_INVITE.body.length).toBeGreaterThan(0);
    expect(DEMO_INVITE.body.length).toBeLessThanOrEqual(500);
  });

  it('gives every member a summary the real dictionaries resolve', () => {
    for (const e of DEMO_FEED) {
      expect(buddyHeading(e.summary)).not.toBe('Member');
      expect(e.summary.country).toMatch(/^[a-z]{2}$/);
    }
  });

  it('has the invited member answer, within the 500-character limit', () => {
    expect(DEMO_REPLY.from).toBe(DEMO_INVITE.to);
    expect(DEMO_REPLY.body.length).toBeGreaterThan(0);
    expect(DEMO_REPLY.body.length).toBeLessThanOrEqual(500);
  });
});
