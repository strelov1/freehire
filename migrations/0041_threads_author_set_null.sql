-- Community threads survive their author's account deletion, de-authored.
--
-- threads.author_user_id was NOT NULL with ON DELETE CASCADE, so erasing a member
-- deleted every thread they opened — and with it every reply other members wrote
-- inside those threads. That is other people's content, and it is not the departing
-- member's to erase. thread_replies.author_user_id has been ON DELETE SET NULL from
-- the start; this brings threads in line with it.
--
-- After this, a thread with no author is a normal state: the read queries LEFT JOIN
-- community_personas and the API renders a deleted-member marker (distinct from the
-- AI persona) in place of a handle. The persona row itself still cascades away, so
-- the pseudonymous handle disappears with the account.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume
-- init, so on a persistent volume this does not auto-apply. Unlike the usual
-- column-addition migrations, deploying the binary first does not error — it
-- silently keeps the old CASCADE, so the first account deletion destroys other
-- members' replies. That is unrecoverable, so run this BEFORE the deploy.

ALTER TABLE public.threads ALTER COLUMN author_user_id DROP NOT NULL;

ALTER TABLE public.threads DROP CONSTRAINT threads_author_user_id_fkey;

ALTER TABLE public.threads
    ADD CONSTRAINT threads_author_user_id_fkey FOREIGN KEY (author_user_id)
    REFERENCES public.users(id) ON DELETE SET NULL;
