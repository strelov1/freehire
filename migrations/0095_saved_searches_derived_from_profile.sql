-- Marks a saved search as auto-generated from the owner's candidate profile (the
-- "notify me about jobs matching my profile" toggle). The partial unique index is
-- the server-owned invariant: at most one such row exists per user, so the create
-- endpoint and the profile page's toggle never have to guess which row is "the"
-- profile search.

ALTER TABLE public.saved_searches ADD COLUMN derived_from_profile boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX saved_searches_derived_from_profile_idx
    ON public.saved_searches (user_id)
    WHERE derived_from_profile;
