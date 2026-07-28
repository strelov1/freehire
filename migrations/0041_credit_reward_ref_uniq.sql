-- DB-level idempotency for AI-credit rewards: at most one 'reward' entry per
-- (user, ref). RewardExists + InsertReward is a check-then-act pair, so two
-- concurrent reward transactions for the same ref could both observe "not yet
-- granted" and double-insert. The debit path already has this guard
-- (credit_ledger_debit_ref_uniq); this mirrors it for rewards.
--
-- Applied to a fresh volume by initdb after 0040; on an existing prod volume
-- build it CONCURRENTLY out of band (a plain CREATE UNIQUE INDEX would lock the
-- live credit_ledger table):
--   CREATE UNIQUE INDEX CONCURRENTLY credit_ledger_reward_ref_uniq
--     ON public.credit_ledger (user_id, ref) WHERE kind = 'reward';
CREATE UNIQUE INDEX IF NOT EXISTS credit_ledger_reward_ref_uniq
    ON public.credit_ledger (user_id, ref) WHERE kind = 'reward';
