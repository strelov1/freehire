-- Drop the now-dead subject_norm column + its index. The inbox switched from a
-- subject-grouped listing to a flat one, so nothing reads subject_norm anymore.
--
-- DROP is the reverse of an add/rename: apply it AFTER deploying the binary that
-- stops writing the column (the new UpsertEmail/InsertHostedMessage omit it; the
-- column's NOT NULL DEFAULT '' covers any old binary still running). There is no
-- 42703 window either way — reads never referenced it.

DROP INDEX IF EXISTS emails_user_subject_idx;
ALTER TABLE emails DROP COLUMN IF EXISTS subject_norm;
