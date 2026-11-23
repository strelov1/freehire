-- Whether every auto-apply attempt waits for the candidate's approval. Off by default: an
-- attempt whose form holds no question only the candidate can answer is sent as soon as
-- its preview is ready. 62 attempts sat unreviewed for over a week when every one waited.

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS auto_apply_confirm_each boolean NOT NULL DEFAULT false;
