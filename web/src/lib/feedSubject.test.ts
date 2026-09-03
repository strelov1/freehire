import { describe, it, expect } from 'vitest';
import { feedSubjectLine } from './feedSubject';
import type { CommunityFeedThread } from './types';

function thread(over: Partial<CommunityFeedThread>): CommunityFeedThread {
  return {
    id: 1,
    subject_type: 'job',
    subject_slug: 'senior-go-engineer-acme-abc123',
    title: 'Is this real?',
    body: 'asking',
    author: 'keen-otter-40',
    reply_count: 0,
    status: 'open',
    created_at: '2026-09-01T00:00:00Z',
    subject_title: 'Senior Go Engineer',
    subject_company: 'Acme Inc.',
    ...over,
  };
}

const companyThread = thread({
  subject_type: 'company',
  subject_slug: 'acme',
  // The server sends the company name in BOTH fields for a company subject.
  subject_title: 'Acme Inc.',
  subject_company: 'Acme Inc.',
});

describe('feedSubjectLine', () => {
  it('reads kind, employer, then posting for a vacancy thread', () => {
    expect(feedSubjectLine(thread({}))).toEqual({
      kind: 'Vacancy',
      employer: 'Acme Inc.',
      posting: 'Senior Go Engineer',
      resolved: true,
    });
  });

  it('omits the posting for a company thread, whose subject is the company', () => {
    // Without this the company name would print twice on one line.
    expect(feedSubjectLine(companyThread)).toEqual({
      kind: 'Company',
      employer: 'Acme Inc.',
      posting: '',
      resolved: true,
    });
  });

  it('marks the kind on both types, not only the ambiguous one', () => {
    expect(feedSubjectLine(thread({})).kind).toBe('Vacancy');
    expect(feedSubjectLine(companyThread).kind).toBe('Company');
  });

  // A posting with an empty jobs.company is a real state in this catalogue, and it is
  // NOT an unresolved subject: the vacancy is there, only its employer name is missing.
  it('names an employerless vacancy without calling the subject unresolved', () => {
    expect(feedSubjectLine(thread({ subject_company: '' }))).toEqual({
      kind: 'Vacancy',
      employer: 'Unknown company',
      posting: 'Senior Go Engineer',
      resolved: true,
    });
  });

  it('falls back to the slug when the subject no longer resolves, and says so', () => {
    // A thread outlives its subject: no FK binds them and cmd/prune hard-deletes
    // vacancies, so the server returns the row with both names empty. `resolved:
    // false` is what stops the slug being rendered as if it were a company's name.
    expect(feedSubjectLine(thread({ subject_title: '', subject_company: '' }))).toEqual({
      kind: 'Vacancy',
      employer: 'senior-go-engineer-acme-abc123',
      posting: '',
      resolved: false,
    });
  });
});
