-- Migration 0182 queued a job on search_outbox whenever its description chunks were written,
-- so the vectors would reach the jobs index's `desc` embedder. That embedder is gone: the
-- search document no longer carries the chunks, so each re-queue was a Meilisearch write
-- that changed nothing. job_semantic_chunks itself stays (cmd/embed still fills it).
DROP TRIGGER IF EXISTS job_semantic_chunks_enqueue_search ON public.job_semantic_chunks;
DROP FUNCTION IF EXISTS public.job_semantic_chunks_enqueue_search();
