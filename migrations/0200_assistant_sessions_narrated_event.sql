-- The last auto_apply_events row an 'apply' session has narrated (see the auto-apply-chat
-- change). The sync call narrates only events newer than this and advances it in the same
-- write that stores the narrating turn, so a failed turn leaves it where it was and the next
-- sync narrates the same events again. NULL: nothing narrated yet, and for every other preset.
--
-- No foreign key: the cursor is a position in the attempt's history, compared by id, and an
-- event row outliving or predating its session changes nothing about that comparison.
-- A metadata-only ALTER.
ALTER TABLE public.assistant_sessions ADD COLUMN narrated_event_id bigint;
