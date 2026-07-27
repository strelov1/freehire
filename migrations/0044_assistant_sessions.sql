-- Assistant sessions and transcripts (see the replace-assistant-runtime change).
-- The in-app assistant used to run as an external process (a roy daemon spawning
-- Claude Code on the user's own machine), which held sessions in its own store and
-- enforced ownership at its HTTP boundary. The agent now runs inside this backend,
-- so a session is an ordinary owned row and ownership is a WHERE user_id = $1.
--
-- assistant_sessions — one conversation. preset selects the system prompt and the
-- tool set: 'chat' is the general job-search assistant, 'tailor' is bound to a
-- tailored CV and its vacancy and additionally gets the CV tools. cv_id/job_id are
-- NULL for a chat session and set for a tailoring one; both cascade/clear with
-- their subject so a session never outlives the CV it edits. label is the sidebar
-- name (the conversation's first user message), NULL until the first turn.
--
-- assistant_messages — the transcript, and at the same time the model's history:
-- one row per message in the conversation, INCLUDING the assistant's tool calls
-- and each tool's result, so a resumed session replays into the model exactly as
-- it happened. content is the message payload as JSONB rather than text because a
-- tool call and a tool result are structured, not prose. seq orders the
-- conversation within its session and is assigned by the writer.
--
-- Note on ids: cvs.agent_session_id stays `text` and now carries this table's id
-- rendered as a decimal string. A CV still holding an old roy UUID simply fails to
-- resolve, which the workspace already handles by starting a fresh session.
--
-- Applied to a fresh volume by initdb after 0043; on an existing prod volume run
-- these statements manually (SET ROLE hire) BEFORE deploying code that reads them.

CREATE TABLE public.assistant_sessions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    preset text NOT NULL,
    label text,
    cv_id bigint,
    job_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT assistant_sessions_preset_check CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text])))
);

ALTER TABLE public.assistant_sessions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.assistant_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_cv_id_fkey FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE SET NULL;

-- The session rail: a caller's sessions, most recently active first. Leading with
-- user_id also keeps account deletion off a sequential scan.
CREATE INDEX assistant_sessions_user_updated_idx
    ON public.assistant_sessions (user_id, updated_at DESC, id DESC);

CREATE TABLE public.assistant_messages (
    session_id bigint NOT NULL,
    seq integer NOT NULL,
    role text NOT NULL,
    content jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT assistant_messages_role_check CHECK ((role = ANY (ARRAY['system'::text, 'user'::text, 'assistant'::text, 'tool'::text])))
);

-- (session_id, seq) is the natural key: the transcript is always read whole, in
-- order, for one session, and the primary key alone serves both.
ALTER TABLE ONLY public.assistant_messages
    ADD CONSTRAINT assistant_messages_pkey PRIMARY KEY (session_id, seq);

ALTER TABLE ONLY public.assistant_messages
    ADD CONSTRAINT assistant_messages_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.assistant_sessions(id) ON DELETE CASCADE;
