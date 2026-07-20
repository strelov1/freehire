-- Referral LinkedIn profiles: both sides of a referral now carry a LinkedIn URL so the
-- other party can vet the person. A referrer's profile backs their "I work here" offer
-- (helps the moderator verify employment); a seeker's profile is shown to the referrer
-- in the inbox alongside the contact channels.
--
-- Required at submission is enforced in the domain layer (URL shape + presence), not by a
-- DB NOT NULL: existing rows predate the column, so the column defaults to '' and legacy
-- rows keep it empty. New submissions always carry a validated value.
--
-- Applied to a fresh volume by initdb after 0035; on an existing prod volume run these
-- statements manually BEFORE deploying code that reads the column.

ALTER TABLE public.referral_offers ADD COLUMN linkedin_url text DEFAULT '' NOT NULL;

ALTER TABLE public.referral_requests ADD COLUMN linkedin_url text DEFAULT '' NOT NULL;
