-- Per-feature, per-day allowance accounting: what a user consumed today, and the
-- append-only record it is derived from. See the add-plan-limits change.
--
-- usage_ledger is the source of truth: one row per consumption, release or grant.
-- usage_daily is a materialised counter read on the hot path, so the gate never sums the
-- ledger, and reconstructable from it at any time.
--
-- NEW TABLES BESIDE credit_ledger RATHER THAN A RESHAPE OF IT. The obvious move is to
-- widen credit_ledger's feature CHECK and swap its period from 'YYYY-MM' to a day. That
-- move leaves one column holding two incompatible meanings: every historical row would
-- carry a month in a field the new code reads as a day, and the banking rule that keeps
-- a reward above the monthly grant would apply to rows that no longer describe a balance.
-- Migrating those rows by calling each month's balance "day one of that month" is worse
-- still — it writes a fact that never happened into an append-only ledger, which is the
-- kind of lie nobody can detect a year later. So credit_ledger and credit_balances are
-- left exactly as they are, written by nobody, read only by account deletion, and dropped
-- in a later change once the new path is trusted.
--
-- day is a date rather than a text period. credit_ledger stored 'YYYY-MM' as text because
-- a month is not a date; a day is one, and the type that knows it rejects the typo that
-- text would accept.
--
-- The feature column carries no CHECK. credit_ledger's admitted only ('match','tailor'),
-- which is exactly why metering the assistant needed a migration at all — the constraint
-- turned "meter one more thing" into a schema change. The set of metered features is a
-- product decision that moves, and the plan configuration in Go is where it is decided;
-- a second copy here could only ever disagree with it.

CREATE TABLE public.usage_ledger (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    feature text NOT NULL,
    day date NOT NULL,
    kind text NOT NULL,
    delta bigint NOT NULL,
    ref text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT usage_ledger_pkey PRIMARY KEY (id),
    CONSTRAINT usage_ledger_kind_check CHECK (kind IN ('consume', 'release', 'grant'))
);

ALTER TABLE public.usage_ledger ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.usage_ledger_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.usage_ledger
    ADD CONSTRAINT usage_ledger_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- Idempotency: at most one live consumption per (user, feature, ref). A retried request,
-- a reconnected stream, a recompute of work already paid for — all re-hit this key and
-- consume nothing further.
--
-- The ref is what makes the tailoring turn ceiling work without a second counter. A
-- session's first ceiling is charged under '<session_id>#1' and buying another under
-- '#2', so extending a session is a distinct event that is still idempotent — a
-- double-clicked "continue" consumes one allowance rather than two. A bare session id
-- could only ever be charged once, which is right for starting and wrong for extending.
--
-- Scoped to kind='consume' so a release, which records the give-back rather than erasing
-- the take, is not constrained by it.
CREATE UNIQUE INDEX usage_ledger_consume_ref_uniq
    ON public.usage_ledger (user_id, feature, ref) WHERE kind = 'consume';

-- The history read: one user's entries, newest first.
CREATE INDEX usage_ledger_user_id_created_at_idx ON public.usage_ledger USING btree (user_id, created_at DESC);

-- Reconstructing a day's counters from the ledger, and reading one session's charges to
-- find the tailoring ceiling in force.
CREATE INDEX usage_ledger_user_feature_day_idx ON public.usage_ledger USING btree (user_id, feature, day);

-- One counter per user, feature and day — the primary key is declared with the table
-- rather than added after it, because a key added to a table that already exists takes an
-- ACCESS EXCLUSIVE lock, and a reader cannot tell from the statement alone that the table
-- it locks was created three lines earlier and is empty.
CREATE TABLE public.usage_daily (
    user_id bigint NOT NULL,
    feature text NOT NULL,
    day date NOT NULL,
    used bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT usage_daily_pkey PRIMARY KEY (user_id, feature, day)
);

ALTER TABLE ONLY public.usage_daily
    ADD CONSTRAINT usage_daily_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
