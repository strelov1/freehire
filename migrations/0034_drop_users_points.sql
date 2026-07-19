-- Drop the legacy users.points counter. It was a separate per-user reward balance
-- (added in 0025), incremented +1 per accepted board contribution. It is superseded
-- by the AI-credits ledger: a contribution now rewards +5 AI credits via credits.Reward
-- (0032), which is the single unit the app surfaces. The counter is dead data — no code
-- reads or writes it after this change — so it is removed rather than left dormant.
--
-- Applied to a fresh volume by initdb after 0033; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code whose users query no longer selects it.

ALTER TABLE public.users DROP COLUMN points;
