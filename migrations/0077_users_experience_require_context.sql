-- Per-user opt-in: interactive experience-atom creates (POST /me/experience/atoms and
-- experience_add) refuse an empty context when this is true. Default false so existing
-- behaviour is unchanged until the owner agrees in chat. Import, owner update, and merge
-- stay ungated regardless of this flag.
--
-- Expansive: the next binary reads this column on experience_add, get_profile, and
-- POST /me/experience/atoms. On an existing prod volume this ALTER must be applied
-- BEFORE that deploy — an unapplied column is a 42703 on those paths, not a degraded
-- feature. A rollback leaves the column harmlessly behind (DEFAULT false).
ALTER TABLE public.users
    ADD COLUMN experience_require_context boolean NOT NULL DEFAULT false;
