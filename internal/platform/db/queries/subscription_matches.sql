-- name: ClaimSubscriptionMatches :many
-- Lease pending, live matches for active subscriptions by stamping claimed_at,
-- AT MOST per_subscription of them per subscription, so one busy subscription cannot
-- starve the rest. FOR UPDATE OF the match rows with SKIP LOCKED lets overlapping passes
-- take disjoint rows so a digest is sent at most once; the lease predicate reclaims rows
-- whose sender died (stale claimed_at), so no separate reaper is needed. The digest is
-- sent OUTSIDE this claim's transaction, so no network call is held inside a row lock.
--
-- The per-subscription cap is the whole point. This used to be one flat
-- `ORDER BY subscription_id, matched_at LIMIT batch_size` over every pending row, which
-- reads as "oldest first" but is really "lowest subscription id first": a subscription
-- whose filter matches most of the catalogue fills the batch every pass and every
-- higher id is never reached. Measured on prod 2026-09-04 — one subscription with an
-- EMPTY query held 248k pending matches, the queue was 1.14M deep, and subscriptions
-- above it had attempts=0 since the day they were created. Nothing was broken; they were
-- simply never in a batch.
--
-- A LATERAL per subscription, rather than a window function, because FOR UPDATE cannot
-- be used with window functions — and the row lock is what makes concurrent passes safe.
--
-- NEWEST first within the subscription, and matched_at is returned so the caller knows
-- where the claim stopped. A digest carries what fits and COUNTS the rest (see
-- CountCoveredMatches / MarkCoveredMatchesNotified), so what fits has to be the freshest.
-- This used to be oldest first, and a `daily` subscription matching more than one digest
-- holds a day never caught up: it mailed the oldest 200 every morning, three weeks behind
-- (subscription 256 on 2026-10-22 was being sent matches from 2026-10-01).
--
-- Stale matches are not filtered here: ExpireStaleSubscriptionMatches runs first in every
-- pass and takes them out of the pending set, so the predicate stays the plain pending one
-- the partial index subscription_matches_pending_matched_idx is built on.
WITH claimable AS (
    SELECT m.subscription_id, m.job_id, s.user_id, s.channel, pl.last_sent AS person_last_sent_at
    FROM subscriptions s
    JOIN saved_searches ss ON ss.id = s.saved_search_id
    JOIN users u ON u.id = s.user_id
    -- Joined BEFORE the lateral so the interval gate below references only relations
    -- that exist under it: the planner then applies the gate at this join, and the
    -- lateral's FOR UPDATE never locks a non-due subscription's rows only for them to
    -- be filtered out afterwards.
    LEFT JOIN notification_settings ns ON ns.user_id = s.user_id
    -- When this person was last written to on this channel: the newest stamp among
    -- their active subscriptions on it (a sent message stamps every subscription it
    -- carried). A webhook keeps its own. Returned too, because the Go side times a
    -- `daily` message by it — the claimed subscriptions alone would read a brand-new
    -- alert as "never sent today" and mail a second digest the same day.
    CROSS JOIN LATERAL (
        SELECT max(o.last_digest_sent_at) AS last_sent
        FROM subscriptions o
        WHERE o.user_id = s.user_id AND o.channel = s.channel AND o.active
          AND (s.channel <> 'webhook' OR o.id = s.id)
    ) pl
    CROSS JOIN LATERAL (
        SELECT mm.subscription_id, mm.job_id
        FROM subscription_matches mm
        WHERE mm.subscription_id = s.id
          AND mm.notified_at IS NULL
          AND mm.failed_at IS NULL
          AND mm.expired_at IS NULL
          AND mm.query_revision = ss.query_revision
          AND (mm.claimed_at IS NULL
               OR mm.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
        ORDER BY mm.matched_at DESC, mm.job_id DESC
        LIMIT sqlc.arg(per_subscription)
        FOR UPDATE SKIP LOCKED
    ) m
    WHERE s.active
      -- Only the subscriptions the caller found deliverable NOW (ListDeliveryCandidates run
      -- through notify's deliveryHold). Without it a daily digest before its time, quiet
      -- hours or an unlinked channel were claimed and released again on every pass, one
      -- UPDATE pair per pending match.
      AND s.id = ANY(sqlc.arg(subscription_ids)::bigint[])
      -- The account-wide "job alerts by email" switch, the same gate ListActiveSubscriptions
      -- applies when matches are recorded. Recording alone is not enough: matches recorded
      -- BEFORE an opt-out (an RFC 8058 one-click unsubscribe) are still pending, and without
      -- this predicate the next digest mails them to a user who has just unsubscribed. They
      -- stay unclaimed and age out through ExpireStaleSubscriptionMatches. Email only: the
      -- switch does not govern Telegram, push or webhook.
      AND (s.channel <> 'email' OR COALESCE(ns.alerts_email_enabled, true))
      -- An 'interval' subscription (no settings row reads as 'daily' since migration
      -- 0246; the interval default is one hour) is claimed only once its interval has elapsed since the last digest. The
      -- gate lives HERE rather than beside the quiet-hours check in deliverMessage because a
      -- subscription that is not due would otherwise be claimed and released on every
      -- five-minute pass: measured on prod 2026-11-04 at ~23k matches an hour, that is
      -- ~12 lease-and-release writes per pending match for nothing. This predicate is the
      -- only definition of "interval elapsed" — the Go side does not repeat it. The
      -- defaults must agree with GetSubscriptionForDelivery's COALESCE and the column
      -- defaults (migrations 0233, 0246).
      --
      -- The interval is the PERSON's, per channel, not each subscription's: the caller
      -- sends one message per (user, channel) carrying every due subscription, and
      -- stamps all of them, so the newest stamp among a person's subscriptions on the
      -- channel is when they were last written to. Gating each subscription on its own
      -- stamp let a user with seven alerts receive seven mails an hour (2026-11-05:
      -- 15 users over 24 a day, one at 59). Measuring the group also keeps a NEW
      -- subscription (no stamp yet) from firing on its own between the hourly mails.
      -- A webhook is sent one subscription at a time (its receiver is a program reading
      -- one saved search), so it keeps its own stamp.
      --
      -- The frequency is the EFFECTIVE one (see ListActiveSubscriptions): a Free account is
      -- daily, which Go times before the claim, so an interval preference that outlived its
      -- entitlement is not gated here and cannot be claimed hourly.
      AND ((CASE WHEN s.channel = 'webhook'
                      OR COALESCE(u.pro_until > now(), false) OR COALESCE(u.ultra_until > now(), false)
                 THEN COALESCE(ns.digest_frequency, 'daily') ELSE 'daily' END) <> 'interval'
           OR COALESCE(pl.last_sent, '-infinity')
              <= now() - make_interval(hours => COALESCE(ns.digest_interval_hours, 1)::int))
    -- A backstop, not the working bound: per_subscription x active subscriptions is what
    -- a pass normally claims. This only caps a pathological subscription count.
    LIMIT sqlc.arg(batch_size)
)
UPDATE subscription_matches m
SET claimed_at = now()
FROM claimable c
WHERE m.subscription_id = c.subscription_id AND m.job_id = c.job_id
RETURNING m.subscription_id, m.job_id, m.matched_at, c.user_id, c.channel, c.person_last_sent_at::timestamptz AS person_last_sent_at;

-- name: ExpireStaleSubscriptionMatches :execrows
-- Take up to batch_size STALE matches out of the pending set by stamping expired_at: a
-- match older than the freshness window, or one whose job has closed. A stale match is
-- never delivered — nobody wants a three-week-old posting announced as new, least of all
-- one that has already closed — and it is neither notified (nobody was told) nor failed
-- (nothing was attempted), so it gets a terminal state of its own (migration 0193).
--
-- This is what bounds the pending set for a subscription that cannot be delivered at all
-- (push with no device, an unlinked Telegram chat, a disabled webhook): its matches used
-- to wait forever, and a user who relinked would have been sent the OLDEST 200 of them.
--
-- Bounded per statement so the first pass after deploy — a backlog of tens of thousands —
-- is a series of short statements rather than one long one holding row locks on a busy
-- host; the caller repeats it until a batch comes back short. A match currently leased by
-- another pass is left to that pass, and SKIP LOCKED steps over one being claimed now.
WITH stale AS (
    SELECT m.subscription_id, m.job_id
    FROM subscription_matches m
    JOIN jobs j ON j.id = m.job_id
    JOIN subscriptions s ON s.id = m.subscription_id
    JOIN saved_searches ss ON ss.id = s.saved_search_id
    WHERE m.notified_at IS NULL
      AND m.failed_at IS NULL
      AND m.expired_at IS NULL
      AND (m.claimed_at IS NULL
           OR m.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
      AND (m.matched_at < now() - make_interval(secs => sqlc.arg(freshness_seconds)::int)
           OR j.closed_at IS NOT NULL
           OR m.query_revision <> ss.query_revision)
    LIMIT sqlc.arg(batch_size)
    FOR UPDATE OF m SKIP LOCKED
)
UPDATE subscription_matches m
SET expired_at = now()
FROM stale s
WHERE m.subscription_id = s.subscription_id AND m.job_id = s.job_id;

-- name: CountCoveredMatches :one
-- How many pending matches a digest will COVER without carrying: the subscription's
-- pending matches strictly older than the oldest one the claim took. The claim reads
-- newest first, so these are exactly the ones that did not fit; the digest counts them in
-- its Total and its "and N more" tail, and MarkCoveredMatchesNotified stamps the same set
-- once the digest has gone out. The two statements share this predicate and must keep
-- sharing it, or the digest would announce one number and retire another.
--
-- (matched_at, job_id) as a row comparison because one RecordSubscriptionMatches batch
-- stamps every row it writes with the same now(): matched_at alone ties constantly.
--
-- Deliberately NO lease predicate. A lease is measured against now(), and the send sits
-- between the count and the mark, so a lease lapsing during a slow send would let the
-- mark retire a row the count left out. Without it the two read the same set; a row
-- another (overlapping, hand-run) pass happens to hold is counted here and delivered
-- there, which is a repeat, not a lie.
SELECT count(*)
FROM subscription_matches m
JOIN subscriptions s ON s.id = m.subscription_id
JOIN saved_searches ss ON ss.id = s.saved_search_id
WHERE m.subscription_id = sqlc.arg(subscription_id)
  AND m.notified_at IS NULL
  AND m.failed_at IS NULL
  AND m.expired_at IS NULL
  AND m.query_revision = ss.query_revision
  AND (m.matched_at, m.job_id) < (sqlc.arg(before_matched_at)::timestamptz, sqlc.arg(before_job_id)::bigint);

-- name: MarkCoveredMatchesNotified :execrows
-- Stamp notified_at on the matches a delivered digest counted without carrying — the set
-- CountCoveredMatches measured before the send, under the same predicate. notified_at is
-- honest here: the digest the person received announced these postings in its count and
-- linked to the saved search that lists them. Re-queuing them instead is what kept a
-- busy `daily` subscription mailing the same backlog, oldest first, day after day.
UPDATE subscription_matches m
SET notified_at = now()
FROM subscriptions s, saved_searches ss
WHERE s.id = m.subscription_id
  AND ss.id = s.saved_search_id
  AND m.subscription_id = sqlc.arg(subscription_id)
  AND m.notified_at IS NULL
  AND m.failed_at IS NULL
  AND m.expired_at IS NULL
  AND m.query_revision = ss.query_revision
  AND (m.matched_at, m.job_id) < (sqlc.arg(before_matched_at)::timestamptz, sqlc.arg(before_job_id)::bigint);
