import { describe, expect, it } from 'vitest';

import { APPLICATION_EVENT_KINDS } from './generated/contracts';
import { eventLabel, eventTone, KIND_LABEL } from './events';
import type { TimelineEvent } from './types';

const event = (kind: string, signal?: string): TimelineEvent =>
  ({ id: 1, kind, signal, source: 'mail_gmail', observed: true, occurred_at: '2026-08-01T09:00:00Z', company_slug: 'acme' }) as TimelineEvent;

// The panel and the calendar render the same ledger. A kind the Go side knows and this map
// does not would fall through to the sentence-cased fallback — which reads plausibly and
// silently drops what the label was meant to add ("Employer replied — rejection" becomes
// "Employer reply"). The fallback is for a server NEWER than this build, not for a kind we
// simply forgot.
describe('KIND_LABEL', () => {
  it('names every kind the contract carries', () => {
    for (const kind of APPLICATION_EVENT_KINDS) {
      expect(KIND_LABEL[kind], kind).toBeDefined();
    }
  });
});

describe('eventLabel', () => {
  it('reads the signal into the sentence where there is one', () => {
    expect(eventLabel(event('employer_reply', 'rejection'))).toBe('Employer replied — rejection');
    expect(eventLabel(event('stage_set', 'interview'))).toBe('Moved to interview');
  });

  it('says the plain thing where there is no signal', () => {
    expect(eventLabel(event('employer_reply'))).toBe('Employer replied');
    expect(eventLabel(event('stage_set'))).toBe('Stage changed');
    expect(eventLabel(event('applied'))).toBe('Applied');
    expect(eventLabel(event('follow_up_sent'))).toBe('Followed up');
    expect(eventLabel(event('interview_scheduled'))).toBe('Interview scheduled');
  });

  // A server ahead of this build can name a kind it has never heard of. Sentence-casing it
  // reads as words rather than as a column name, which is better than a blank row.
  it('sentence-cases a kind from a newer server', () => {
    expect(eventLabel(event('offer_withdrawn'))).toBe('Offer withdrawn');
  });
});

describe('eventTone', () => {
  it('gives every known kind a tone', () => {
    for (const kind of APPLICATION_EVENT_KINDS) {
      expect(eventTone(kind), kind).toMatch(/^text-/);
    }
  });

  it('falls back quietly for an unknown kind', () => {
    expect(eventTone('offer_withdrawn')).toBe('text-muted-foreground');
  });
});
