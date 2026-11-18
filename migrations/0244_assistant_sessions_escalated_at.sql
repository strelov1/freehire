-- When a support session was handed to a human. Written by a conditional UPDATE that is the
-- claim for the one email a session may send, and counted per account to cap how many a day.
-- Nullable and without a default: only support sessions ever set it.
ALTER TABLE public.assistant_sessions ADD COLUMN escalated_at timestamptz;
