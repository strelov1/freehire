-- The credential a social channel publishes with, when that credential EXPIRES.
--
-- The daily digest's first channel needed no such table: a Discord incoming webhook URL is
-- itself the credential, it never expires, and so it lives in .env beside every other static
-- secret. LinkedIn's does expire — an access token lasts 60 days — and that one difference is
-- what forces storage the process can WRITE. A worker that renews a token cannot put the new
-- value back into .env, so the token has to live where a write is ordinary.
--
-- Stored as plaintext, unlike gmail_tokens (0014) and mobile refresh tokens (0087), and the
-- difference is whose credential it is. Those hold hundreds of tokens belonging to USERS, where
-- a leaked dump means reading strangers' mail; this holds exactly one credential belonging to
-- us, of the same sensitivity as DISCORD_DIGEST_WEBHOOK_URL sitting in cleartext in .env two
-- lines away. Encrypting it would buy nothing that the database's own access control does not
-- already give, and would cost a fourth secret — one whose loss would make the stored token
-- unreadable while looking exactly like a broken channel.
--
-- Keyed by channel rather than holding one LinkedIn-shaped row, for the same reason
-- social_digest_posts is: what makes a second destination cheap is that the schema already
-- names the destination. A hypothetical Mastodon token is then a Go change and a config line.
CREATE TABLE public.social_tokens (
    -- Matches social_digest_posts.channel — 'linkedin' is the only value today. The two are
    -- not foreign-keyed to each other: one records what we published, the other how we may
    -- publish, and a channel can hold a credential having published nothing.
    channel            text        NOT NULL PRIMARY KEY,

    access_token       text        NOT NULL,
    -- When the access token dies. NOT NULL because a token whose expiry we do not know is a
    -- token we cannot renew ahead of time, and renewing ahead of time is this table's whole
    -- reason to exist. The OAuth response always carries expires_in, so there is no case to
    -- represent.
    access_expires_at  timestamptz NOT NULL,

    -- The refresh token, if LinkedIn gave us one. NULLABLE ON PURPOSE, and the null is the
    -- expected case rather than the exceptional one: programmatic refresh tokens are issued
    -- only to approved Marketing Developer Platform partners, and the Community Management
    -- API is not that program. When it is absent the renewal worker cannot renew and instead
    -- warns, in time for a person to sign in again — see cmd/linkedin-token-refresh.
    refresh_token      text,
    refresh_expires_at timestamptz,

    -- The scopes the token actually carries, as LinkedIn returned them. Recorded because a
    -- token that posts (w_organization_social) and one that only reads look identical until
    -- the first 403, and that 403 would arrive at 06:45 UTC in a worker nobody is watching.
    scope              text        NOT NULL DEFAULT '',

    -- When a person last completed the sign-in that minted this credential, and when a worker
    -- last renewed it. Two columns and not one: the first bounds how long the refresh token
    -- has left (365 days from the sign-in, and a renewal does NOT extend it), the second is
    -- how an operator tells a channel that is renewing itself from one that is quietly
    -- riding out its last 60 days.
    obtained_at        timestamptz NOT NULL DEFAULT now(),
    refreshed_at       timestamptz
);

COMMENT ON TABLE public.social_tokens IS
    'The expiring credential a social publisher posts with, one row per channel. Written by '
    'cmd/linkedin-auth (a person signs in) and cmd/linkedin-token-refresh (a worker renews). '
    'Read by cmd/social-digest. Plaintext on purpose: this is our own service credential, of '
    'the same sensitivity as the webhook URL in .env, not a user''s.';

COMMENT ON COLUMN public.social_tokens.refresh_token IS
    'NULL is the expected case: LinkedIn issues programmatic refresh tokens only to approved '
    'Marketing Developer Platform partners. With no refresh token the renewal worker warns '
    'ahead of expiry instead of renewing, and a person re-runs the sign-in.';
