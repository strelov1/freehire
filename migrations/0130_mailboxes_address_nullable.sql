-- squawk-ignore-file ban-drop-not-null -- intentional: address is retired in favor of the derived <username>@<domain> address, existing rows are untouched, and no client reads NULL-vs-empty here (Go decodes the column as pgtype.Text). The per-line `-- squawk-ignore` form does not suppress this rule for a multi-line ALTER TABLE / ALTER COLUMN statement in squawk-cli 2.63.0 (verified empirically, same class of issue as 0128's prefer-robust-stmts), hence the file-level form.
--
-- Hosted-mailbox addresses are no longer allocated or stored independently (see the
-- add-username-claim change): the effective address is always
-- "<users.username>@<MAIL_DOMAIN>", computed at read time. New mailboxes rows
-- therefore no longer carry an address, so the NOT NULL this column has carried since
-- 0015_hosted_mailbox.sql must go — a plain ALTER COLUMN DROP NOT NULL is
-- metadata-only regardless of table size (no scan), and mailboxes is a small,
-- opt-in-feature table besides.
--
-- The UNIQUE constraint stays: Postgres treats multiple NULLs as distinct for
-- uniqueness purposes, so it neither blocks new NULL rows nor stops meaning anything
-- for the historical rows that still carry their original address. The column itself
-- is left in place — dropped in a later cleanup migration once new code has run in
-- production for a while (see the add-username-claim change's design.md).
ALTER TABLE public.mailboxes
    ALTER COLUMN address DROP NOT NULL;
