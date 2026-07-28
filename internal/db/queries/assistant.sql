-- name: CreateAssistantSession :one
-- Start a conversation for a user. preset selects the prompt and tool set ('chat' or
-- 'tailor'); cv_id/job_id bind a tailoring session to its CV and vacancy and are NULL
-- for a chat. The label is set later, from the first user message.
INSERT INTO assistant_sessions (user_id, preset, cv_id, job_id)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, preset, label, cv_id, job_id, created_at, updated_at;

-- name: ListAssistantChatSessions :many
-- The caller's session rail: their general chats and the browsing conversations they held
-- from the extension's side panel, most recently active first. Owner-scoped by construction
-- — another user's sessions can never appear. A browsing session belongs here because it is
-- one the candidate can pick up at their desk; it simply cannot see a page from there.
-- Tailoring conversations are deliberately excluded: they belong to the CV that owns them
-- and are reached through the tailoring workspace, so listing them here would put a chat in
-- the rail that leads nowhere useful and cannot be continued without its CV.
SELECT id, user_id, preset, label, cv_id, job_id, created_at, updated_at
FROM assistant_sessions
WHERE user_id = $1 AND preset IN ('chat', 'browse')
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
UPDATE assistant_sessions
SET updated_at = now()
WHERE id = $1;

-- name: SetAssistantSessionLabel :exec
-- Name a session from its first user message. Applied only while the label is still unset,
-- so a long conversation keeps the name it was born with.
UPDATE assistant_sessions
SET label = $2
WHERE id = $1 AND label IS NULL;

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
-- A session's whole transcript in order. It is both what the client replays and what the
-- model's history is rebuilt from, so tool calls and tool results are included.
SELECT session_id, seq, role, content, created_at
FROM assistant_messages
WHERE session_id = $1
ORDER BY seq;
