-- The user's own verdict on a support conversation: true "it helped", false "it did not", NULL
-- not rated. The only signal of how well the support agent answers short of reading every
-- transcript. Nullable and without a default, so adding it rewrites nothing; only support
-- sessions ever set it, and a later vote overwrites an earlier one.
ALTER TABLE public.assistant_sessions ADD COLUMN IF NOT EXISTS helpful boolean;
