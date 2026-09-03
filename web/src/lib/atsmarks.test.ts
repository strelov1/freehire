import { describe, expect, it } from 'vitest';

import { ATS_MARKS } from './atsmarks';

describe('ATS brand marks', () => {
  it('carries the Greenhouse mark, drawable and named', () => {
    const mark = ATS_MARKS.greenhouse;

    expect(mark?.title).toBe('Greenhouse');
    expect(mark?.path).toBeTruthy();
    expect(mark?.hex).toBe('24A47F');
  });

  // Asserted rather than left implicit. The caption puts the mark BESIDE the
  // provider's name precisely because four of the five platforms we capture have
  // none, and a change that quietly added a wrong mark for one of them would
  // otherwise look like coverage improving.
  it.each(['ashby', 'workable', 'lever', 'recruitee'])('knows no mark for %s', (provider) => {
    expect(ATS_MARKS[provider]).toBeUndefined();
  });

  it('knows no mark for a platform it has never heard of', () => {
    expect(ATS_MARKS.teamtailor).toBeUndefined();
  });
});
