-- The Talent Network catalogue handle: the one public identifier of a member's
-- card, at /talent/<handle>. See openspec/changes/talent-network-public-catalog.
--
-- NULLABLE, and that is the design, not an omission. A handle is minted the first
-- time a candidate joins and never changes afterwards; an account that never joins
-- has none. A DEFAULT would mint one for every account in the table — including the
-- overwhelming majority who will never be members — and a NOT NULL would force it.
--
-- Deliberately NOT the account's `username`. internal/identity/username derives that
-- from the email's local part, so for most accounts it is the person's own name, and
-- the hosted mailbox adopts the same string as an address. Either one in the URL of a
-- page that exists to withhold the name would undo the feature in the address bar.
--
-- No transaction marker: ADD COLUMN with no default is metadata-only on PG11+, so it
-- takes ACCESS EXCLUSIVE only for the instant it takes to write the catalogue entry,
-- and the runner's default transaction around a single instantaneous statement costs
-- nothing. Contrast 0085's uuid column, whose volatile default forced a full rewrite.
--
-- Uniqueness is a separate file (0148), because CREATE INDEX CONCURRENTLY cannot run
-- inside a transaction and this one may.
--
-- Applied to a fresh volume by initdb after 0146; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying the code that reads it.

ALTER TABLE public.users
    ADD COLUMN talent_handle text;
