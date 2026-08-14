import { describe, expect, it } from 'vitest';
import { threadMatchesSubject } from './threadSubject';

describe('threadMatchesSubject', () => {
  it('matches when both the subject kind and slug agree with the URL', () => {
    const thread = { subject_type: 'company', subject_slug: 'acme' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(true);
  });

  it('rejects a thread that resolves under a different slug of the same kind', () => {
    // A syntactically valid threadId can name a real thread that belongs to a
    // DIFFERENT company than the one in the URL — getThread(id) alone has no way
    // to know that.
    const thread = { subject_type: 'company', subject_slug: 'other-co' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(false);
  });

  it('rejects a thread that resolves under the other subject kind entirely', () => {
    // Thread ids are not namespaced per subject kind, so a job's thread id can
    // collide with a company detail page's [threadId] param.
    const thread = { subject_type: 'job', subject_slug: 'acme' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(false);
  });
});
