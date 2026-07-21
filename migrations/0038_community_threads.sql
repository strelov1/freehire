-- Community discussion threads: an anonymous, polymorphic discussion primitive
-- (see the add-community-threads change). A signed-in user starts a topic attached
-- to a subject and any signed-in user replies; the author is shown only through a
-- stable pseudonymous persona, never their real identity.
--
-- community_personas — one stable handle per user, minted lazily on their first
-- authored thread or reply and reused forever. The handle is the ONLY author
-- identity ever sent to a client; user_id is the private key the handle hides
-- behind (it powers moderation and rate limiting). ON DELETE CASCADE drops a
-- user's persona with the user.
--
-- threads — a topic attached to a subject. The subject is polymorphic:
-- (subject_type, subject_ref) where subject_ref is the subject's PUBLIC SLUG
-- (companies.slug for 'company', jobs.public_slug for 'job'). A text ref is used,
-- not a bigint id, because the two subjects have heterogeneous keys — companies'
-- primary key IS its slug, while jobs has a bigint id plus a unique public_slug —
-- and the slug is the one key both share (and the key the API addresses them by).
-- There is deliberately no FK to the subject: the primitive stays decoupled so a
-- future subject_type plugs in with no schema change. anchor_path is a nullable
-- seam (unused now) for a future subject that anchors a thread to a sub-part of
-- itself (e.g. a CV bullet). reply_count is denormalized so the subject listing
-- never COUNT(*)s replies. status gates the hot listing and locks replies.
--
-- thread_replies — chronological replies, optionally nested: parent_reply_id is a
-- self-reference (NULL = a top-level reply on the thread, non-NULL = a reply to
-- another reply), so the client builds the comment tree. author_user_id is nullable
-- and is_ai a seam for a future AI participant (no AI posts in this change). Votes
-- are intentionally omitted — additive later.
--
-- Applied to a fresh volume by initdb after 0037; on an existing prod volume run
-- these statements manually (SET ROLE hire) BEFORE deploying code that reads them.

CREATE TABLE public.community_personas (
    user_id bigint NOT NULL,
    handle text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.community_personas
    ADD CONSTRAINT community_personas_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.community_personas
    ADD CONSTRAINT community_personas_handle_key UNIQUE (handle);

ALTER TABLE ONLY public.community_personas
    ADD CONSTRAINT community_personas_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.threads (
    id bigint NOT NULL,
    subject_type text NOT NULL,
    subject_ref text NOT NULL,
    anchor_path text,
    title text NOT NULL,
    body text NOT NULL,
    author_user_id bigint NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT threads_subject_type_check CHECK ((subject_type = ANY (ARRAY['company'::text, 'job'::text]))),
    CONSTRAINT threads_status_check CHECK ((status = ANY (ARRAY['open'::text, 'closed'::text])))
);

ALTER TABLE public.threads ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.threads_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.threads
    ADD CONSTRAINT threads_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.threads
    ADD CONSTRAINT threads_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- The subject listing: a subject's open threads, newest first. Partial on
-- status='open' so closed/hidden threads never enter the hot index.
CREATE INDEX threads_subject_open_created_idx
    ON public.threads (subject_type, subject_ref, created_at DESC, id DESC)
    WHERE status = 'open';

-- Rate limiting: how many threads a user opened in a recent window.
CREATE INDEX threads_author_created_idx
    ON public.threads (author_user_id, created_at);

CREATE TABLE public.thread_replies (
    id bigint NOT NULL,
    thread_id bigint NOT NULL,
    parent_reply_id bigint,
    author_user_id bigint,
    is_ai boolean DEFAULT false NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.thread_replies ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.thread_replies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.thread_replies
    ADD CONSTRAINT thread_replies_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.thread_replies
    ADD CONSTRAINT thread_replies_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.threads(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.thread_replies
    ADD CONSTRAINT thread_replies_author_user_id_fkey FOREIGN KEY (author_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

-- A nested reply points at its parent reply; deleting a reply removes its subtree.
ALTER TABLE ONLY public.thread_replies
    ADD CONSTRAINT thread_replies_parent_reply_id_fkey FOREIGN KEY (parent_reply_id) REFERENCES public.thread_replies(id) ON DELETE CASCADE;

-- Reply reads: a thread's replies in chronological order.
CREATE INDEX thread_replies_thread_created_idx
    ON public.thread_replies (thread_id, created_at);

-- Rate limiting: how many replies a user posted in a recent window.
CREATE INDEX thread_replies_author_created_idx
    ON public.thread_replies (author_user_id, created_at);
