-- Community discussion threads (see the add-community-threads change). Read paths
-- join community_personas so a row carries the author's handle, never their user_id.

-- name: GetCommunityPersona :one
-- A user's stable pseudonymous handle, or no row when they have never posted.
SELECT * FROM community_personas WHERE user_id = $1;

-- name: InsertCommunityPersona :one
-- Mint a persona. ON CONFLICT (user_id) DO NOTHING makes a concurrent same-user mint
-- return no row (the repository re-reads the winner); a handle-unique violation is a
-- different collision the repository maps to a retry.
INSERT INTO community_personas (user_id, handle)
VALUES ($1, $2)
ON CONFLICT (user_id) DO NOTHING
RETURNING *;

-- name: InsertThread :one
-- Open a thread. The author's handle is filled by the service from the minted
-- persona, so no join is needed here.
INSERT INTO threads (subject_type, subject_ref, title, body, author_user_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetCommunityThread :one
-- A single thread with its author handle.
SELECT t.*, p.handle AS author_handle
FROM threads t
JOIN community_personas p ON p.user_id = t.author_user_id
WHERE t.id = $1;

-- name: ListOpenThreadsFirst :many
-- First page of a subject's open threads, newest first. Served by the partial index
-- threads_subject_open_created_idx.
SELECT t.*, p.handle AS author_handle
FROM threads t
JOIN community_personas p ON p.user_id = t.author_user_id
WHERE t.subject_type = $1 AND t.subject_ref = $2 AND t.status = 'open'
ORDER BY t.created_at DESC, t.id DESC
LIMIT $3;

-- name: ListOpenThreadsAfter :many
-- Keyset continuation: rows strictly older than the cursor (created_at, id). No
-- OFFSET, so deep pages never scan skipped rows.
SELECT t.*, p.handle AS author_handle
FROM threads t
JOIN community_personas p ON p.user_id = t.author_user_id
WHERE t.subject_type = $1 AND t.subject_ref = $2 AND t.status = 'open'
  AND (t.created_at < sqlc.arg(cursor_created_at)
       OR (t.created_at = sqlc.arg(cursor_created_at) AND t.id < sqlc.arg(cursor_id)))
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(page_limit);

-- name: CountOpenThreadsBySubject :one
-- Open-thread count for one subject — the "Discussion · N" badge on the detail page.
-- Served by the partial index threads_subject_open_created_idx; scoped to a single
-- subject so it stays cheap (not the cross-subject count the design rules out).
SELECT count(*) FROM threads
WHERE subject_type = $1 AND subject_ref = $2 AND status = 'open';

-- name: CloseCommunityThread :exec
-- Moderator close: the thread leaves the open listing and rejects new replies.
UPDATE threads SET status = 'closed' WHERE id = $1;

-- name: InsertThreadReply :one
-- parent_reply_id is NULL for a top-level reply, or another reply's id to nest under it.
INSERT INTO thread_replies (thread_id, parent_reply_id, author_user_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: IncrementThreadReplyCount :exec
UPDATE threads SET reply_count = reply_count + 1 WHERE id = $1;

-- name: ListThreadRepliesFirst :many
-- First page of a thread's replies, oldest first. LEFT JOIN so a future AI reply
-- (null author) still returns, with a null handle the API renders as the AI persona.
SELECT r.*, p.handle AS author_handle
FROM thread_replies r
LEFT JOIN community_personas p ON p.user_id = r.author_user_id
WHERE r.thread_id = $1
ORDER BY r.created_at ASC, r.id ASC
LIMIT $2;

-- name: ListThreadRepliesAfter :many
SELECT r.*, p.handle AS author_handle
FROM thread_replies r
LEFT JOIN community_personas p ON p.user_id = r.author_user_id
WHERE r.thread_id = $1
  AND (r.created_at > sqlc.arg(cursor_created_at)
       OR (r.created_at = sqlc.arg(cursor_created_at) AND r.id > sqlc.arg(cursor_id)))
ORDER BY r.created_at ASC, r.id ASC
LIMIT sqlc.arg(page_limit);

-- name: CountRecentThreadsByUser :one
-- Rate-limit count: threads a user opened since a cutoff. Served by
-- threads_author_created_idx.
SELECT count(*) FROM threads WHERE author_user_id = $1 AND created_at > $2;

-- name: CountRecentRepliesByUser :one
SELECT count(*) FROM thread_replies WHERE author_user_id = $1 AND created_at > $2;
