-- name: UpsertScreeningAnswer :exec
-- Records the candidate's answer to one screening question, replacing any earlier answer on
-- the same topic. The question text is refreshed too: the newest wording is the one they
-- most recently read and answered, and keeping a stale phrasing beside a fresh answer would
-- misdescribe what was agreed to.
--
-- provenance is overwritten on conflict rather than preserved: a candidate answering a
-- question themselves supersedes any earlier suggestion, and that is exactly the promotion
-- the send-gate depends on.
--
-- The hazard is the OTHER direction, and this statement does not guard it: an agent write
-- would equally overwrite a candidate's own answer, DEMOTING it out of what may be sent
-- (internal/candidate/answerbank.Provenance.sendable) and replacing text they authored with
-- a model's reading. Nothing writes agent_inferred today, so the behaviour is unreachable
-- and stays as it is rather than being guarded speculatively. Whoever adds that writer owns
-- this: either the statement grows a `WHERE screening_answer_bank.provenance <> 'candidate'`
-- guard on the agent path, or the agent's suggestions go somewhere that is not this row.
INSERT INTO screening_answer_bank (user_id, topic, question, answer, provenance)
VALUES (sqlc.arg(user_id), sqlc.arg(topic), sqlc.arg(question), sqlc.arg(answer), sqlc.arg(provenance))
ON CONFLICT (user_id, topic) DO UPDATE
SET question   = EXCLUDED.question,
    answer     = EXCLUDED.answer,
    provenance = EXCLUDED.provenance,
    updated_at = now();

-- name: ListScreeningAnswers :many
-- One candidate's whole bank, newest first — what the management surface lists and what the
-- profile assembler merges into the answer map.
SELECT id, user_id, topic, question, answer, provenance, created_at, updated_at
FROM screening_answer_bank
WHERE user_id = sqlc.arg(user_id)
ORDER BY updated_at DESC, id DESC;

-- name: DeleteScreeningAnswer :execrows
-- The owner removes one answer. Scoped by user_id as well as id, so a foreign id affects
-- zero rows and the handler renders 404 — never revealing to a probing caller which of the
-- two it was, the same posture GetAutoApplyQueueEntryForReview already takes.
--
-- Nothing else ever deletes from this table: the bank accumulates, and no reconciler prunes
-- it. Same rule as the experience bank, and for the same reason — a sweeper here would
-- silently discard answers the candidate expects to still hold.
DELETE FROM screening_answer_bank
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);
