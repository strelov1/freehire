-- AI credits: a unified per-user points balance that the metered AI features
-- (résumé match, CV tailoring) draw from. See the add-ai-credits change.
--
-- credit_ledger is the append-only source of truth: one row per grant or debit.
-- Balances are derived from it. The monthly grant (kind='grant', feature NULL)
-- lands once per (user, period); each metered action appends a debit (negative
-- delta) tagged with its feature and a ref (job_id for match, cv_id for tailor).
-- The kind CHECK admits a future 'purchase' grant, and the grant-uniqueness index
-- is scoped to kind='grant' so purchases are not constrained to one-per-period.
--
-- credit_balances is a materialized cache of the current period's remaining
-- points (one row per user), read on the hot debit path so the gate never sums
-- the ledger. It is reconstructable from credit_ledger at any time.
--
-- Applied to a fresh volume by initdb after 0030; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the tables.

CREATE TABLE public.credit_ledger (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    period text NOT NULL,
    kind text NOT NULL,
    feature text,
    delta integer NOT NULL,
    ref text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credit_ledger_kind_check CHECK (kind IN ('grant', 'debit', 'purchase')),
    CONSTRAINT credit_ledger_feature_check CHECK (feature IS NULL OR feature IN ('match', 'tailor'))
);

ALTER TABLE public.credit_ledger ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.credit_ledger_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.credit_ledger
    ADD CONSTRAINT credit_ledger_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.credit_ledger
    ADD CONSTRAINT credit_ledger_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- Idempotency: at most one debit per (user, feature, ref). A recompute of the same
-- job or a resume of the same tailored CV re-hits this key and consumes nothing.
CREATE UNIQUE INDEX credit_ledger_debit_ref_uniq
    ON public.credit_ledger (user_id, feature, ref) WHERE kind = 'debit';

-- At most one monthly grant per (user, period). Purchase grants are exempt.
CREATE UNIQUE INDEX credit_ledger_grant_period_uniq
    ON public.credit_ledger (user_id, period) WHERE kind = 'grant';

-- The ledger-read path: a user's history newest first.
CREATE INDEX credit_ledger_user_id_created_at_idx ON public.credit_ledger USING btree (user_id, created_at DESC);

CREATE TABLE public.credit_balances (
    user_id bigint NOT NULL,
    period text NOT NULL,
    remaining integer NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.credit_balances
    ADD CONSTRAINT credit_balances_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.credit_balances
    ADD CONSTRAINT credit_balances_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
