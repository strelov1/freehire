-- Referral offers and requests become addressable by random UUIDs (see the
-- opaque-referral-ids change).
--
-- Both were bigint identities, named in /me/referrals/offers/<id>,
-- /me/referrals/incoming/<id>/cv and /referrals/offers/<id>/proof. This is the
-- last user-owned resource still addressed by a counter.
--
-- The stakes are higher here than for a CV, and differently shaped: an incoming
-- request's CV is deliberately served to SOMEONE ELSE — an approved referrer of
-- the company the request is addressed to. That authorization is a membership
-- question, not an ownership one, and the harder a check is, the more expensive
-- it is to get wrong once. A countable id turns one mistake there into "download
-- every attached résumé"; a random one leaves it a single failed request.
--
-- Simpler than 0045: nothing references either table, so there are no dependent
-- columns to carry across and no foreign keys to rebuild. Dropping each column
-- takes its identity sequence with it, which is the point — no counter is left to
-- read. Both CHECK constraints on these tables reference other columns, not id,
-- so they survive; only a CHECK naming the dropped column would vanish with it
-- (the lesson from 0045, where referral_requests' cv_kind rule went quietly).
--
-- Applied to a fresh volume by initdb after 0045; on an existing prod volume run
-- this file manually (SET ROLE hire) BEFORE deploying code that reads it. Run it
-- with a lock_timeout: a DDL request for ACCESS EXCLUSIVE parks at the head of the
-- lock queue, so a blocked migration takes the table's traffic down with it.

BEGIN;

ALTER TABLE public.referral_offers ADD COLUMN new_id uuid DEFAULT gen_random_uuid() NOT NULL;
ALTER TABLE public.referral_offers DROP CONSTRAINT referral_offers_pkey;
ALTER TABLE public.referral_offers DROP COLUMN id;
ALTER TABLE public.referral_offers RENAME COLUMN new_id TO id;
ALTER TABLE public.referral_offers ADD CONSTRAINT referral_offers_pkey PRIMARY KEY (id);

ALTER TABLE public.referral_requests ADD COLUMN new_id uuid DEFAULT gen_random_uuid() NOT NULL;
ALTER TABLE public.referral_requests DROP CONSTRAINT referral_requests_pkey;
ALTER TABLE public.referral_requests DROP COLUMN id;
ALTER TABLE public.referral_requests RENAME COLUMN new_id TO id;
ALTER TABLE public.referral_requests ADD CONSTRAINT referral_requests_pkey PRIMARY KEY (id);

COMMIT;
