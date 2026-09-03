import { describe, expect, it } from 'vitest';
import type { CommunityThread } from '$lib/types';
import { threadMatchesSubject } from './threadSubject';

/** Exactly what the function reads off a thread. Annotated rather than inferred so the
 *  stubs carry the wire type's `subject_type` union instead of widening it to string. */
type Subject = Pick<CommunityThread, 'subject_type' | 'subject_slug'>;

describe('threadMatchesSubject', () => {
  it('matches when both the subject kind and slug agree with the URL', () => {
    const thread: Subject = { subject_type: 'company', subject_slug: 'acme' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(true);
  });

  it('rejects a thread that resolves under a different slug of the same kind', () => {
    // A syntactically valid threadId can name a real thread that belongs to a
    // DIFFERENT company than the one in the URL — getThread(id) alone has no way
    // to know that.
    const thread: Subject = { subject_type: 'company', subject_slug: 'other-co' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(false);
  });

  it('rejects a thread that resolves under the other subject kind entirely', () => {
    // Thread ids are not namespaced per subject kind, so a job's thread id can
    // collide with a company detail page's [threadId] param.
    const thread: Subject = { subject_type: 'job', subject_slug: 'acme' };
    expect(threadMatchesSubject(thread, 'company', 'acme')).toBe(false);
  });
});
