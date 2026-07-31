import { describe, it, expect } from 'vitest';
import { canFollowUp, chasedLabel, clipboardText } from './followup';
import type { MyJob, FollowUpDraft } from './types';

// The card's follow-up logic reads only the silence verdict and the chase mark;
// a minimal cast fixture suffices.
function job(fields: Partial<MyJob>): MyJob {
  return { silence_state: null, days_silent: null, followed_up_at: null, ...fields } as MyJob;
}

const NOW = new Date('2026-07-30T12:00:00Z');
const daysAgo = (n: number) => new Date(NOW.getTime() - n * 86_400_000).toISOString();

describe('canFollowUp', () => {
  it('offers a draft on a silent application', () => {
    expect(canFollowUp(job({ silence_state: 'silent', days_silent: 24 }))).toBe(true);
  });

  it('offers nothing where the board shows no silence badge', () => {
    expect(canFollowUp(job({ silence_state: 'active', days_silent: 3 }))).toBe(false);
    expect(canFollowUp(job({ silence_state: 'unconfirmed', days_silent: 30 }))).toBe(false);
    expect(canFollowUp(job({ silence_state: null }))).toBe(false);
  });

  // The server refuses anything that is not `silent`, so an offer the server would
  // reject is a button that 409s. Already having chased is not such a case: the
  // employer still has not replied, and a second chase is the candidate's call.
  it('keeps offering after a chase was recorded', () => {
    expect(canFollowUp(job({ silence_state: 'silent', days_silent: 24, followed_up_at: daysAgo(2) }))).toBe(true);
  });
});

describe('chasedLabel', () => {
  it('says nothing for an application never chased', () => {
    expect(chasedLabel(job({ silence_state: 'silent', days_silent: 24 }), NOW)).toBeNull();
  });

  it('reports the chase in the same day unit as the silence badge', () => {
    expect(chasedLabel(job({ followed_up_at: daysAgo(2) }), NOW)).toBe('chased 2d ago');
    expect(chasedLabel(job({ followed_up_at: daysAgo(45) }), NOW)).toBe('chased 45d ago');
  });

  it('reads naturally on the day of the chase', () => {
    expect(chasedLabel(job({ followed_up_at: daysAgo(0) }), NOW)).toBe('chased today');
  });

  // A clock skew between server and browser must not print "chased -1d ago".
  it('floors a future timestamp at today', () => {
    expect(chasedLabel(job({ followed_up_at: daysAgo(-1) }), NOW)).toBe('chased today');
  });

  it('ignores an unparseable timestamp rather than rendering NaN', () => {
    expect(chasedLabel(job({ followed_up_at: 'not-a-date' }), NOW)).toBeNull();
  });
});

describe('clipboardText', () => {
  const draft: FollowUpDraft = {
    subject: 'Following up: Go Engineer application',
    body: 'Hello,\n\nI applied…',
    days_silent: 24,
  };

  // The draft is pasted into a mail client, where the subject is a separate field.
  // Labelling it keeps it from silently becoming the first line of the body.
  it('carries the subject labelled, above the body', () => {
    expect(clipboardText(draft)).toBe(
      'Subject: Following up: Go Engineer application\n\nHello,\n\nI applied…',
    );
  });

  it('adds the recipient only when one is known', () => {
    expect(clipboardText({ ...draft, recipient: 'jane@acme.com' })).toBe(
      'To: jane@acme.com\nSubject: Following up: Go Engineer application\n\nHello,\n\nI applied…',
    );
  });
});
