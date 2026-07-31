import { describe, expect, it } from 'vitest';
import { canRehearse } from './rehearsal';
import type { MyJob } from './types';

function application(stage: string | null): MyJob {
  return { stage, job: { title: 'Go Developer', company: 'Acme' } } as unknown as MyJob;
}

describe('canRehearse', () => {
  // The rehearsal is worth its cost where an interview is actually coming: a call has
  // been offered or one has happened. Everything earlier is a different problem.
  it('offers a rehearsal once an interview is in play', () => {
    expect(canRehearse(application('screening'))).toBe(true);
    expect(canRehearse(application('interview'))).toBe(true);
  });

  it('stays out of the way before anyone has replied', () => {
    expect(canRehearse(application('applied'))).toBe(false);
    expect(canRehearse(application(null))).toBe(false);
  });

  // A settled application has no interview left to rehearse, and offering one on a
  // rejection would be the cruellest button on the board.
  it('does not offer one on a settled application', () => {
    expect(canRehearse(application('offer'))).toBe(false);
    expect(canRehearse(application('rejected'))).toBe(false);
  });
});
