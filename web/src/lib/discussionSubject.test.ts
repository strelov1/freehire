import { describe, it, expect } from 'vitest';
import { companySubject, jobSubject } from './discussionSubject';

describe('jobSubject', () => {
  it('reads the posting title, its employer, and an open state', () => {
    expect(jobSubject({ title: 'Senior Go Engineer', company: 'Acme Inc.' })).toEqual({
      title: 'Senior Go Engineer',
      company: 'Acme Inc.',
      employerLabel: 'Acme Inc.',
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

  // A posting with no recorded employer is a real state in this catalogue. It has a
  // label to print, and NO logo key: the proxy resolves by name and would go fetch a
  // mark for the literal "Unknown company".
  it('labels an employerless vacancy without giving the logo a name to resolve', () => {
    expect(jobSubject({ title: 'Platform Engineer', company: '' })).toEqual({
      title: 'Platform Engineer',
      company: '',
      employerLabel: 'Unknown company',
      closed: false,
    });
  });
});

describe('companySubject', () => {
  it('uses the name as heading, logo key and label, and never closes', () => {
    // The caller prints the name once for a company: title and employer are the same
    // string, and there is no closed state for an employer.
    expect(companySubject({ name: 'Acme Inc.' })).toEqual({
      title: 'Acme Inc.',
      company: 'Acme Inc.',
      employerLabel: 'Acme Inc.',
      closed: false,
    });
  });
});
