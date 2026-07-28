-- The experience bank (see the experience-bank change).
--
-- Until now everything the product learned about what a candidate has actually
-- done was either discarded or overwritten. users.resume_structured is a cache of
-- the uploaded file — a re-upload replaces it, and nothing can be added to it — so
-- an achievement a candidate confirmed to the tailoring agent survived only inside
-- that session's transcript. These two tables are the durable home for that
-- knowledge: additive across re-uploads and sessions, and removed only by the user.
--
-- experience_employments — a place where something happened: a job or a project.
-- Seeded from the uploaded CV's work history and extended by hand. Dates are kept
-- as free-form labels exactly as printed on the CV ("2021-03", "Mar 2021",
-- "Present") — no parsing is attempted, matching resumeextract.Experience and
-- cv.ExperienceItem. There is deliberately NO unique constraint on
-- (user_id, company, role): it would make import a clean upsert, but it would also
-- forbid a real career shape — returning to the same employer, in the same role,
-- years later. Import matches in code instead, and the cost of that choice is a
-- possible duplicate the user can delete.
--
-- experience_atoms — one piece of evidence, at the grain of a CV bullet. claim is
-- the sentence; context is how it was done, kept as raw material for reframing
-- against a vacancy's language. skills holds canonical slugs from internal/skilltag
-- — the SAME dictionary that produces the jobs.skills facet — so "does this
-- candidate have evidence for this requirement" is a set intersection rather than a
-- text-matching problem.
--
-- provenance is the honest wall, moved out of the system prompt and into the data:
-- cv_import / stated_in_chat / manual may be rendered into a CV bullet;
-- agent_inferred is a legal thing for the agent to record and an illegal thing to
-- publish, until the user confirms it and it is re-stamped. Enforcement lives in
-- the service layer, but the vocabulary is pinned here so an unknown value cannot
-- be persisted at all.
--
-- claim_key is the normalized claim (lowercased, punctuation and whitespace
-- collapsed) that import matches on. It is a stored column rather than an
-- expression so the uniqueness is a real constraint: re-uploading the same CV
-- cannot duplicate atoms, no matter what the import code does.
--
-- Deleting an employment cascades to its atoms — they are evidence OF that role,
-- and a user removing a job from their history expects its bullets to go with it.
--
-- Applied to a fresh volume by initdb after 0046; on an existing prod volume run
-- these statements manually (SET ROLE hire) BEFORE deploying code that reads them.

CREATE TABLE public.experience_employments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    kind text NOT NULL,
    company text DEFAULT ''::text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    location text DEFAULT ''::text NOT NULL,
    period_start text DEFAULT ''::text NOT NULL,
    period_end text DEFAULT ''::text NOT NULL,
    is_current boolean DEFAULT false NOT NULL,
    summary text DEFAULT ''::text NOT NULL,
    -- The role's technologies, as printed on the CV's per-role stack line. Kept on the
    -- employment rather than copied onto each of its atoms: a bullet about hiring would
    -- otherwise carry a Kafka tag. Retrieval reads both — an atom whose own text never
    -- names MongoDB still surfaces for a MongoDB requirement when the role ran on it,
    -- just with less weight than one that names it.
    stack text[] DEFAULT '{}'::text[] NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT experience_employments_kind_check CHECK ((kind = ANY (ARRAY['job'::text, 'project'::text])))
);

ALTER TABLE ONLY public.experience_employments
    ADD CONSTRAINT experience_employments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.experience_employments
    ADD CONSTRAINT experience_employments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- The bank is always read for one owner, most recent role first. Leading with
-- user_id also keeps account deletion off a sequential scan.
CREATE INDEX experience_employments_user_idx
    ON public.experience_employments (user_id, is_current DESC, period_start DESC);

CREATE TABLE public.experience_atoms (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    employment_id uuid,
    claim text NOT NULL,
    claim_key text NOT NULL,
    context text DEFAULT ''::text NOT NULL,
    metrics text[] DEFAULT '{}'::text[] NOT NULL,
    skills text[] DEFAULT '{}'::text[] NOT NULL,
    provenance text NOT NULL,
    source_ref text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT experience_atoms_provenance_check CHECK ((provenance = ANY (ARRAY['cv_import'::text, 'stated_in_chat'::text, 'manual'::text, 'agent_inferred'::text])))
);

ALTER TABLE ONLY public.experience_atoms
    ADD CONSTRAINT experience_atoms_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.experience_atoms
    ADD CONSTRAINT experience_atoms_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.experience_atoms
    ADD CONSTRAINT experience_atoms_employment_id_fkey FOREIGN KEY (employment_id) REFERENCES public.experience_employments(id) ON DELETE CASCADE;

-- Import's dedup guarantee, as a constraint rather than a hope: the same claim
-- cannot be banked twice for one user.
CREATE UNIQUE INDEX experience_atoms_user_claim_key
    ON public.experience_atoms (user_id, claim_key);

-- Grouping atoms under their employment, and the owner-scoped scan retrieval walks.
--
-- There is deliberately NO inverted index on skills. Retrieval scores canonical
-- skill-set intersection first, which sounds like a GIN case — but it must also
-- return evidence a requirement matches on text alone ("led a team of five" carries
-- no skill slug), so it reads every atom the owner has and scores in Go. A GIN index
-- would be indexing for a query nobody makes. If a prefilter ever earns its place,
-- adding it is one migration.
CREATE INDEX experience_atoms_user_employment_idx
    ON public.experience_atoms (user_id, employment_id);
