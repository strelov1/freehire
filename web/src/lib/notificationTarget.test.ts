import { describe, it, expect } from 'vitest';
import { notificationTarget } from './notificationTarget';

describe('notificationTarget', () => {
  it('sends a reminder to its job', () => {
    expect(notificationTarget({ kind: 'reminder', public_slug: 'acme-go-engineer' })).toEqual({
      kind: 'job',
      slug: 'acme-go-engineer',
    });
  });

  it('sends a job-closed nudge to its job', () => {
    expect(notificationTarget({ kind: 'nudge_job_closed', public_slug: 'acme-go-engineer' })).toEqual({
      kind: 'job',
      slug: 'acme-go-engineer',
    });
  });

  it('sends a single-job subscription digest to its job', () => {
    expect(notificationTarget({ kind: 'subscription_digest', public_slug: 'acme-go-engineer' })).toEqual({
      kind: 'job',
      slug: 'acme-go-engineer',
    });
  });

  it('sends a multi-job digest to its own jobs-list page', () => {
    expect(
      notificationTarget({
        id: 42,
        kind: 'subscription_digest',
        public_slug: null,
        jobs: [{ title: 'A', company: 'Acme', slug: 'a' }],
      }),
    ).toEqual({ kind: 'digest', id: 42 });
  });

  it('sends a digest with neither a slug nor a jobs snapshot nowhere (defensive)', () => {
    expect(notificationTarget({ id: 42, kind: 'subscription_digest', public_slug: null, jobs: null })).toEqual({
      kind: 'none',
    });
  });

  // The two nudge kinds the tracking board itself is about — even though the row
  // still carries the job's slug (design.md decision 5), web links to the board.
  it('sends a follow-up nudge to the tracking board, not the job', () => {
    expect(notificationTarget({ kind: 'nudge_follow_up', public_slug: 'acme-go-engineer' })).toEqual({
      kind: 'tracking',
    });
  });

  it('sends an interview-prep nudge to the tracking board, not the job', () => {
    expect(notificationTarget({ kind: 'nudge_interview_prep', public_slug: 'acme-go-engineer' })).toEqual({
      kind: 'tracking',
    });
  });

  // A nudge always carries a slug in practice, but the function is slug-first: no
  // slug means nothing to open, even for a kind that would otherwise route to the
  // tracking board.
  it('sends nowhere when a nudge kind somehow has no slug', () => {
    expect(notificationTarget({ kind: 'nudge_follow_up', public_slug: null })).toEqual({ kind: 'none' });
  });
});
