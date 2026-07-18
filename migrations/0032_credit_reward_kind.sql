-- Reward AI credits for accepted board contributions: widen the credit_ledger kind
-- CHECK to admit 'reward' entries (a positive, non-expiring grant, feature NULL, keyed
-- by the contribution id). Rewards bank above the monthly grant and carry over — the
-- lazy period reset floors the balance at the grant but preserves any banked surplus.
-- See the credits-contribution-reward change.
--
-- Applied to a fresh volume by initdb after 0031; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that inserts 'reward' rows.

ALTER TABLE public.credit_ledger DROP CONSTRAINT credit_ledger_kind_check;

ALTER TABLE public.credit_ledger
    ADD CONSTRAINT credit_ledger_kind_check CHECK (kind IN ('grant', 'debit', 'purchase', 'reward'));
