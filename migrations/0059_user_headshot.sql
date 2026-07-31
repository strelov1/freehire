-- The per-user headshot pointer: photo_object_key names the object in the same bucket
-- as the stored CV (blobstore.PhotoKey — "photos/<id>", derived from the user id, never
-- from client input), photo_uploaded_at stamps when it was last replaced. Parity with
-- resume_object_key / resume_uploaded_at from 0001: the bytes live in object storage,
-- the row only points at them, so a bucket that is unconfigured leaves the column NULL
-- and the feature simply reports itself unavailable.
--
-- The upload time is not decoration — the SPA appends it to the image URL as a cache
-- buster, which is what lets the photo be served with a normal private cache instead of
-- no-store while a replacement still shows up immediately.
--
-- Two nullable columns: no table rewrite, no default to backfill, no lock of consequence
-- (contrast 0011). Additive and unread by the previous binary, so the deploy order does
-- not matter and a rollback leaves the columns harmlessly behind.
ALTER TABLE public.users
    ADD COLUMN photo_object_key text,
    ADD COLUMN photo_uploaded_at timestamp with time zone;
