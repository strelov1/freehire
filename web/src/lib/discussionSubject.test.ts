import { describe, it, expect } from 'vitest';
import { companySubject, jobSubject } from './discussionSubject';

describe('jobSubject', () => {
  it('reads the posting title, its employer, and an open state', () => {
    expect(jobSubject({ title: 'Senior Go Engineer', company: 'Acme Inc.' })).toEqual({
      kind: 'job',
      title: 'Senior Go Engineer',
      company: 'Acme Inc.',
      closed: false,
    });
  });

  // A vacancy's closed state is the question its discussions are usually asking, so
  // it is derived here rather than left for a reader to infer from the linked page.
  it('marks a closed vacancy closed', () => {
    expect(
      jobSubject({
        title: 'Senior Go Engineer',
        company: 'Acme Inc.',
        closed_at: '2026-07-24T19:44:45Z',
      }).closed,
    ).toBe(true);
  });
});

describe('companySubject', () => {
  it('uses the name as both the heading and the logo key, and never closes', () => {
    // The caller prints the name once for a company: title and company are the same
    // string, and there is no closed state for an employer.
    expect(companySubject({ name: 'Acme Inc.' })).toEqual({
      kind: 'company',
      title: 'Acme Inc.',
      company: 'Acme Inc.',
      closed: false,
    });
  });
});
