-- The roy agent session bound to a tailored CV, so the CV list can re-open the exact session
-- instead of starting a new one. NULL for a base CV, or a tailored CV created before this column.
ALTER TABLE cvs ADD COLUMN agent_session_id text;
