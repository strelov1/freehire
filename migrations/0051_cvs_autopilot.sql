-- The autopilot run on a tailored CV (see the tailor-autopilot change): one unattended agent
-- turn that walks the vacancy's requirements, searches the experience bank for each, and
-- rewrites what the evidence supports. Two columns hold everything a run leaves behind.
--
-- autopilot_report is the run's own account of itself: one entry per requirement it considered,
-- carrying an outcome from a fixed vocabulary (closed from the bank, closed from what the
-- candidate said, still open, never reached) and a one-line note. It lives on the CV rather
-- than in the conversation because the workspace panel must render it after a reload without
-- parsing a transcript. The agent replaces it whole on every write; there is no partial update.
--
-- autopilot_undo is the document as it stood before the run's first edit, so the whole run can
-- be reverted in one move. Reverting restores it and clears BOTH columns: a report describing
-- edits that no longer exist misdescribes the CV. A second run overwrites the snapshot, so a
-- revert always returns to the start of the LAST run.
--
-- Both are nullable and unbackfilled — an existing CV simply has no run yet.
--
-- Applied to a fresh volume by initdb after 0050; on an existing prod volume run this file
-- manually (SET ROLE hire) BEFORE deploying code that reads it.

ALTER TABLE cvs ADD COLUMN autopilot_report jsonb;
ALTER TABLE cvs ADD COLUMN autopilot_undo jsonb;
