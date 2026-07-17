-- Link contributions: the crowdsourced "contribute a board" flow (see the
-- add-link-contributions change). A signed-in user pastes a job link from a supported
-- multi-tenant ATS — a vacancy URL or a bare board-listing URL — and the backend derives the
-- company board `(source, board)` from the URL alone (no network). A board we do not yet
-- crawl and that nobody has contributed is recorded here and the submitter is awarded one
-- point (users.points). The ingest side later adds the board to sources and scrapes ALL its
-- vacancies, so the UNIT of a contribution is the board, not a single vacancy.
--
-- The dedup key is the board: UNIQUE (source, board) rejects a second contribution of the
-- same company (whether the second link is another vacancy or the board listing) and makes
-- the concurrent-duplicate race safe (the loser hits the unique violation; its transaction —
-- insert + point — rolls back). `url` keeps the exact link the user pasted for reference.
--
-- status is the seam for the deferred ingest worker that will validate + onboard the board;
-- this flow only ever writes 'pending'.
--
-- Applied to a fresh volume by initdb after 0024; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the table/column.

CREATE TABLE public.link_contributions (
    id bigint NOT NULL,
    submitted_by bigint NOT NULL,
    url text NOT NULL,
    source text NOT NULL,
    board text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT link_contributions_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'onboarded'::text, 'rejected'::text])))
);

ALTER TABLE public.link_contributions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.link_contributions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.link_contributions
    ADD CONSTRAINT link_contributions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.link_contributions
    ADD CONSTRAINT link_contributions_submitted_by_fkey FOREIGN KEY (submitted_by) REFERENCES public.users(id) ON DELETE CASCADE;

-- Board dedup + concurrency guard: one company board can be contributed at most once.
ALTER TABLE ONLY public.link_contributions
    ADD CONSTRAINT link_contributions_source_board_key UNIQUE (source, board);

-- The "my contributions" list: a user's contributions, newest first.
CREATE INDEX link_contributions_submitted_by_created_at_idx ON public.link_contributions USING btree (submitted_by, created_at DESC);

-- points is the per-user reward balance, incremented once per accepted board contribution.
-- Defaults 0 so existing accounts start empty.
ALTER TABLE public.users
    ADD COLUMN points integer DEFAULT 0 NOT NULL;
