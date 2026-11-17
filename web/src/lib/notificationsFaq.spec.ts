import { describe, expect, it } from 'vitest';
import { NOTIFICATIONS_FAQ } from './notificationsFaq';

// The FAQ is also the page's structured data, so it must state the real limits: faster than
// daily is a Pro/Ultra benefit, and free search alerts stay available as a daily digest.
describe('notifications FAQ', () => {
  it('does not promise instant alerts to everyone', () => {
    const answer = NOTIFICATIONS_FAQ[0]?.answer ?? '';
    expect(answer).toMatch(/Free accounts get .*daily digest/);
    expect(answer).toMatch(/Pro and Ultra/);
  });
});
