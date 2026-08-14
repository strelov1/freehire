-- name: CreateAssistantSession :one
-- Start a conversation for a user. preset selects the prompt and tool set ('chat' or
-- 'tailor'); cv_id/job_id bind a tailoring session to its CV and vacancy and are NULL
-- for a chat. The label is set later, from the first user message.
INSERT INTO assistant_sessions (user_id, preset, cv_id, job_id)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, preset, label, cv_id, job_id, created_at, updated_at;

-- name: ListAssistantChatSessions :many
-- The caller's session rail: their unbound conversations, most recently active first.
-- Owner-scoped by construction — another user's sessions can never appear.
--
-- The rail carries every conversation that can be continued on its own: chat, profile,
-- browse, interview and debrief alike. An experience interview is resumable and would
-- otherwise be lost the moment its author navigated away; a browsing conversation begun in
-- the extension's side panel is one the candidate can pick up at their desk, where it
-- simply cannot see a page any more; a rehearsal is opened days before the interview and
-- closed again, and a debrief is written in one sitting and reread before the next round.
--
-- A rehearsal and a debrief are bound to a vacancy, so the test is not "binds to nothing"
-- — it is whether the conversation still works when reopened from here. It does: their
-- context tool closes over the vacancy id the session already carries. Tailoring
-- conversations are excluded for exactly that reason inverted — they belong to the CV that
-- owns them, are reached through the tailoring workspace, and cannot be continued without it.
SELECT id, user_id, preset, label, cv_id, job_id, created_at, updated_at
FROM assistant_sessions
WHERE user_id = $1 AND preset IN ('chat', 'profile', 'browse', 'interview', 'debrief')
ORDER BY updated_at DESC, id DESC;

-- name: GetAssistantSession :one
-- One session owned by the caller. Owner-scoped: a foreign or missing id returns no row,
-- which the handler maps to 404 — so a probe cannot tell the two apart.
SELECT id, user_id, preset, label, cv_id, job_id, created_at, updated_at
FROM assistant_sessions
WHERE id = $1 AND user_id = $2;

-- name: DeleteAssistantSession :execrows
-- Remove an owned session; its transcript goes with it (ON DELETE CASCADE). Returns 0
-- affected rows for a foreign or missing id.
DELETE FROM assistant_sessions
WHERE id = $1 AND user_id = $2;

-- name: TouchAssistantSession :exec
-- Mark a session as the most recently active, so the rail's order follows real use.
-- Owner-scoped like every other write in this file (Get/Delete both require id AND
-- user_id): a bare id would let any caller who learns another user's session id touch
-- it, and every call site already has the owner's id in hand (the Session it just
-- read, created, or otherwise proved ownership of).
UPDATE assistant_sessions
SET updated_at = now()
WHERE id = $1 AND user_id = $2;

-- name: SetAssistantSessionLabel :exec
-- Name a session from its first user message. Applied only while the label is still unset,
-- so a long conversation keeps the name it was born with. Owner-scoped for the same
-- reason TouchAssistantSession is.
UPDATE assistant_sessions
SET label = $3
WHERE id = $1 AND user_id = $2 AND label IS NULL;

-- name: AppendAssistantMessage :one
-- Append one message to a session's transcript, assigning the next sequence number in the
-- same statement so concurrent writers cannot collide on (session_id, seq) — the primary
-- key rejects a duplicate rather than silently reordering the conversation.
INSERT INTO assistant_messages (session_id, seq, role, content)
VALUES (
    $1,
    (SELECT coalesce(max(seq), 0) + 1 FROM assistant_messages WHERE session_id = $1),
    $2,
    $3
)
RETURNING session_id, seq, role, content, created_at;

-- name: ListAssistantMessages :many
-- A session's whole transcript in order, for the client to replay. Tool calls and tool
-- results are included. Unbounded by design: the client's own message list must show
-- everything, not a trimmed window — see ListRecentAssistantMessages for the bounded
-- read the model's own history is rebuilt from.
SELECT session_id, seq, role, content, created_at
FROM assistant_messages
WHERE session_id = $1
ORDER BY seq;

-- name: ListRecentAssistantMessages :many
-- The session's most recent messages, newest first — the bounded counterpart of
-- ListAssistantMessages, for rebuilding the model's own history every turn. Runner.trim()
-- only ever keeps the tail (HistoryLimit, default 60) of what ListAssistantMessages
-- returns; fetching and JSON-decoding the WHOLE transcript first, only to discard
-- everything but the tail, cost time and memory proportional to total session length
-- (autopilot runs, long-lived chat/tailoring sessions can accumulate hundreds of rows)
-- instead of the fixed window actually used. The caller reverses these rows back to
-- ascending seq order before handing them to trim()/Conversation().
SELECT session_id, seq, role, content, created_at
FROM assistant_messages
WHERE session_id = $1
ORDER BY seq DESC
LIMIT $2;
