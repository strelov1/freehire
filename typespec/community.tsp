import "@typespec/http";

using Http;

namespace Freehire;

// Anonymous discussion threads attached to a company or vacancy. Reads are public and
// carry only pseudonymous persona handles, never a user id; writing needs a session
// cookie and closing a thread is for moderators.

/** The author is the persona handle, "AI" for a system reply or "[deleted]" once the account is gone. */
model Thread {
  id: int64;
  subject_type: string;
  subject_slug: string;
  title: string;
  body: string;
  author: string;
  reply_count: int64;
  status: string;
  created_at: utcDateTime;
}

/** A feed row: the thread plus the subject's display names, empty when the subject no longer exists. */
model RecentThread {
  ...Thread;
  subject_title: string;
  subject_company: string;
}

model ThreadReply {
  id: int64;
  thread_id: int64;
  parent_id: int64;
  author: string;
  body: string;
  created_at: utcDateTime;
}

/** `next_cursor` is set only when the page came back full; a partial page is the last. */
model ThreadCursorMeta {
  next_cursor?: string;
}

model ThreadPage {
  data: Thread[];
  meta: ThreadCursorMeta;
}

model RecentThreadPage {
  data: RecentThread[];
  meta: ThreadCursorMeta;
}

model ThreadCount {
  count: int64;
}

model ThreadWithReplies {
  thread: Thread;
  replies: ThreadReply[];
}

model ThreadDetailPage {
  data: ThreadWithReplies;
  meta: ThreadCursorMeta;
}

model ThreadClosed {
  id: int64;
  status: string;
}

model CreateThreadRequest {
  subject_type?: string;
  subject_slug?: string;
  title?: string;
  body?: string;
}

model CreateReplyRequest {
  body?: string;

  /** Nests the reply under another reply; 0 or omitted is top-level. */
  parent_reply_id?: int64;
}

/** A subject's open threads, newest first, keyset-paged. */
@tag("Community")
@route("/threads")
@get
op listThreads(
  @query subject_type?: string,
  @query subject_slug?: string,
  @query cursor?: string,
): ThreadPage | Problem;

/** Open threads across every subject, newest first, keyset-paged: the discussions feed. */
@tag("Community")
@route("/threads/recent")
@get
op listRecentThreads(@query cursor?: string): RecentThreadPage | Problem;

/** A subject's open-thread count. */
@tag("Community")
@route("/threads/count")
@get
op countThreads(@query subject_type?: string, @query subject_slug?: string): Data<ThreadCount> | Problem;

/** A thread with its first page of replies. The id is declared a string and parsed in Go, which keeps the 400 text. */
@tag("Community")
@route("/threads/{id}")
@get
op getThread(@path id: string, @query cursor?: string): ThreadDetailPage | Problem;

/** Opens a thread on a company or job. */
@tag("Community")
@route("/threads")
@post
op createThread(@body body: CreateThreadRequest): {
  @statusCode _: 201;
  @body body: Data<Thread>;
} | Problem;

/** Posts a reply to an open thread. */
@tag("Community")
@route("/threads/{id}/replies")
@post
op createReply(@path id: string, @body body: CreateReplyRequest): {
  @statusCode _: 201;
  @body body: Data<ThreadReply>;
} | Problem;

/** Moderators only: closes a thread. */
@tag("Community")
@route("/threads/{id}/close")
@post
op closeThread(@path id: string): Data<ThreadClosed> | Problem;
