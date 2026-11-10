-- A moderator can ban an account by hand. Nothing here bans automatically: no complaint,
-- bounce or abuse signal writes these columns, only the moderator endpoint.
--
-- banned_at is the whole switch. NULL is a live account. The session check, the API-key
-- and OAuth-grant lookups, sign-in, and every optional mail and push/Telegram/webhook
-- delivery read it, so a ban takes effect on the next request with no cache to expire.
-- Data is kept: a ban hides the account from every credential, it deletes nothing.
--
-- banned_by is the moderator who did it. It is SET NULL when that moderator's account is
-- deleted (the trail outlives the member, like jobs.created_by); the ban itself stays.

ALTER TABLE users
    ADD COLUMN banned_at timestamptz,
    ADD COLUMN ban_reason text,
    -- squawk-ignore adding-foreign-key-constraint -- the column is all NULL and users is ~21k rows; the scan and lock are instant
    ADD COLUMN banned_by bigint REFERENCES users (id) ON DELETE SET NULL,
    -- A ban without a stated reason is not allowed, and an unban clears all three.
    -- squawk-ignore constraint-missing-not-valid -- every existing row has all three NULL, so the scan is instant
    ADD CONSTRAINT users_ban_consistent CHECK (
        (banned_at IS NULL AND ban_reason IS NULL AND banned_by IS NULL)
        OR (banned_at IS NOT NULL AND ban_reason IS NOT NULL AND btrim(ban_reason) <> '')
    );

-- Every column referencing users needs its own index (TestEveryUserForeignKeyIsIndexed).
-- squawk-ignore require-concurrent-index-creation -- users is ~21k rows; the build is instant
CREATE INDEX users_banned_by_idx ON users (banned_by);

-- mail_recipient (0247) with the ban: a banned account is never mailed. The three-argument
-- form stays so a binary still running the previous release keeps working while this
-- migration is applied ahead of it; no query in this release calls it, and a later
-- migration drops it.
CREATE FUNCTION mail_recipient(email text, email_verified boolean, last_active_at timestamptz, banned_at timestamptz) RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE
AS $$ SELECT CASE WHEN banned_at IS NULL AND email_verified AND last_active_at > now() - interval '180 days' THEN email ELSE '' END $$;
