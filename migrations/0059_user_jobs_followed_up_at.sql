-- When the candidate chased a silent application. Set by the record action after they send the
-- follow-up draft; NULL means they never did, which is true of every existing row.
--
-- It is deliberately NOT part of the last-activity derivation in ListUserJobs. That derivation is
-- GREATEST(applied_at, newest linked inbound mail): it measures when the OTHER SIDE last moved, and
-- a chase the candidate sends is not a reply. Folding this column in would make the board report an
-- answer that never came, and would clear the silence badge precisely when the candidate most needs
-- to see it. The card reads both: still silent, and already chased.
ALTER TABLE public.user_jobs ADD COLUMN followed_up_at timestamptz;
