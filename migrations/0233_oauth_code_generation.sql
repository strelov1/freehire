-- Bind a pending OAuth authorization code to the account generation that
-- approved it (the same latch oauth_grants.issued_token_version, 0177, applies
-- to grants). The token exchange must refuse a code minted before a
-- generation-changing revocation — password reset or change, logout-all,
-- verified-identity seizure — instead of stamping the CURRENT users.token_version
-- onto a fresh grant.
--
-- The column is nullable on purpose: a code issued before this migration has NO
-- recoverable consent generation (users.token_version has been free to move many
-- times since its consent), so each pending row is stamped with 0, the
-- unknown-generation sentinel — generations start at 1 (0041's DEFAULT), so 0
-- never matches a live account and the code is refused at redemption, fail
-- closed. The one-time cost is a restart of any consent still pending at deploy
-- (at most 10 minutes of code lifetime): the client restarts its authorize round
-- trip under the new schema. Casting the CURRENT generation onto such a code
-- instead would validate a consent that may predate a revocation — the exact
-- upgrade this column exists to prevent. Note the UPDATE is a one-time backfill
-- only: it installs no default, so rows the old binary writes before the deploy
-- carry NULL, which the exchange's equality latch treats as never-matching —
-- dead in both rollout directions.
ALTER TABLE oauth_authorization_codes
    -- squawk-ignore prefer-bigint-over-int -- mirrors users.token_version (int, 0041_account_security.sql), which this column is compared against
    ADD COLUMN issued_token_version INT;

UPDATE oauth_authorization_codes SET issued_token_version = 0;
