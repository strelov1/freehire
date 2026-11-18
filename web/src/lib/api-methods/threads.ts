import type { Http } from '../api-core';
import type { CommunityFeedThread, CommunityReply, CommunityThread } from '../types';
import { jsonBody } from '../api-core';

export function threadsApi({ request, requestData }: Http) {
  /** A subject's open discussion threads, newest first. `subjectType` is 'company'
   *  or 'job', `subjectSlug` the subject's public slug. `nextCursor` (when present)
   *  fetches the following keyset page. Public — no auth needed to read. */
  async function listThreads(
    subjectType: string,
    subjectSlug: string,
    cursor?: string,
  ): Promise<{ threads: CommunityThread[]; nextCursor?: string }> {
    const qs = new URLSearchParams({ subject_type: subjectType, subject_slug: subjectSlug });
    if (cursor) qs.set('cursor', cursor);
    const res = await request<{ data: CommunityThread[]; meta: { next_cursor?: string } }>(`/api/v1/threads?${qs}`);
    return { threads: res.data, nextCursor: res.meta?.next_cursor };
  }

  /** Open discussion threads across every subject, newest first — the /discussions
   *  feed. Each row names its own subject, since the caller has no other way to know
   *  what a thread is about. Public — no auth needed to read. */
  async function listRecentThreads(cursor?: string): Promise<{ threads: CommunityFeedThread[]; nextCursor?: string }> {
    const qs = new URLSearchParams();
    if (cursor) qs.set('cursor', cursor);
    const suffix = qs.size ? `?${qs}` : '';
    const res = await request<{ data: CommunityFeedThread[]; meta: { next_cursor?: string } }>(
      `/api/v1/threads/recent${suffix}`,
    );
    return { threads: res.data, nextCursor: res.meta?.next_cursor };
  }

  /** How many open discussion threads a subject has — the detail-page badge. Public. */
  async function countThreads(subjectType: string, subjectSlug: string): Promise<number> {
    const qs = new URLSearchParams({ subject_type: subjectType, subject_slug: subjectSlug });
    return (await requestData<{ count: number }>(`/api/v1/threads/count?${qs}`)).count;
  }

  /** A single thread with its first page of replies (oldest first). Public. */
  async function getThread(
    id: number,
    cursor?: string,
  ): Promise<{ thread: CommunityThread; replies: CommunityReply[]; nextCursor?: string }> {
    const suffix = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
    const res = await request<{
      data: { thread: CommunityThread; replies: CommunityReply[] };
      meta: { next_cursor?: string };
    }>(`/api/v1/threads/${id}${suffix}`);
    return { thread: res.data.thread, replies: res.data.replies, nextCursor: res.meta?.next_cursor };
  }

  /** Open a thread on a subject. Requires a signed-in session (401 otherwise). */
  async function createThread(input: {
    subject_type: string;
    subject_slug: string;
    title: string;
    body: string;
  }): Promise<CommunityThread> {
    return requestData<CommunityThread>('/api/v1/threads', jsonBody('POST', input));
  }

  /** Post a reply to a thread. Requires a signed-in session. `parentReplyId` nests
   *  the reply under another reply (0/omitted = a top-level reply). */
  async function createReply(threadId: number, body: string, parentReplyId = 0): Promise<CommunityReply> {
    return requestData<CommunityReply>(
      `/api/v1/threads/${threadId}/replies`,
      jsonBody('POST', { body, parent_reply_id: parentReplyId }),
    );
  }

  return {
    listThreads,
    listRecentThreads,
    countThreads,
    getThread,
    createThread,
    createReply,
  };
}
