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

describe('feedSubjectLine', () => {
  it('reads employer then posting for a vacancy thread', () => {
    expect(feedSubjectLine(thread({}))).toEqual({
      employer: 'Acme Inc.',
      posting: 'Senior Go Engineer',
    });
  });

  it('omits the posting for a company thread, whose subject is the company', () => {
    // The server sends the company name in BOTH fields for a company subject, so
    // without this the name would print twice on one line.
    expect(
      feedSubjectLine(
        thread({
          subject_type: 'company',
          subject_slug: 'acme',
          subject_title: 'Acme Inc.',
          subject_company: 'Acme Inc.',
        }),
      ),
    ).toEqual({ employer: 'Acme Inc.', posting: '' });
  });

  it('falls back to the slug when the subject no longer resolves', () => {
    // A thread outlives its subject: no FK binds them and cmd/prune hard-deletes
    // vacancies, so the server returns the row with both names empty.
    expect(feedSubjectLine(thread({ subject_title: '', subject_company: '' }))).toEqual({
      employer: 'senior-go-engineer-acme-abc123',
      posting: '',
    });
  });
});
