-- Employee referrals: a moderated, anonymous channel connecting a seeker with an
-- insider willing to refer them (see the add-employee-referrals change).
--
-- referral_offers — "I can refer into company X". A member picks a company, uploads
-- a CV as proof of employment (proof_object_key, stored via the resume S3 path), and
-- the offer waits on moderation (status pending → approved / rejected). UNIQUE
-- (user_id, company_slug) allows one offer per member per company and makes the
-- concurrent-duplicate race safe. Only an approved offer makes a company eligible for
-- referral requests. company is referenced by slug — companies' PK is the slug, the
-- same key jobs.company_slug carries.
--
-- referral_requests — "please refer me into company X". A seeker attaches a CV (their
-- stored original resume, cv_kind='original'; or a builder CV, cv_kind='built' + cv_id),
-- leaves a contact (Telegram and/or email — at least one, enforced by CHECK) and a note.
-- The request is company-scoped (a pool): every approved referrer of the company sees
-- it, and whichever one acts records acted_by/acted_at. job_id records the vacancy the
-- seeker came from as context; job_id and cv_id are ON DELETE SET NULL seams so a request
-- outlives the vacancy or CV it referenced (same pattern as cvs.job_id). A partial unique
-- index enforces at most one active (status='sent') request per (seeker, company); once
-- resolved (contacted/declined) the seeker may request again.
--
-- Applied to a fresh volume by initdb after 0033; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the tables.

CREATE TABLE public.referral_offers (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    company_slug text NOT NULL,
    proof_object_key text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_by bigint,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT referral_offers_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);

ALTER TABLE public.referral_offers ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.referral_offers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.referral_offers
    ADD CONSTRAINT referral_offers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.referral_offers
    ADD CONSTRAINT referral_offers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.referral_offers
    ADD CONSTRAINT referral_offers_company_slug_fkey FOREIGN KEY (company_slug) REFERENCES public.companies(slug) ON DELETE CASCADE;

ALTER TABLE ONLY public.referral_offers
    ADD CONSTRAINT referral_offers_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES public.users(id) ON DELETE SET NULL;

-- One offer per member per company; also the concurrent-duplicate guard.
ALTER TABLE ONLY public.referral_offers
    ADD CONSTRAINT referral_offers_user_company_key UNIQUE (user_id, company_slug);

-- Referral-availability lookup + the referrer's inbox join: approved offers by company.
CREATE INDEX referral_offers_company_approved_idx ON public.referral_offers USING btree (company_slug) WHERE (status = 'approved'::text);

-- The "my offers" list: a member's offers, newest first.
CREATE INDEX referral_offers_user_created_at_idx ON public.referral_offers USING btree (user_id, created_at DESC);

-- The moderator queue: pending offers, oldest first.
CREATE INDEX referral_offers_pending_created_at_idx ON public.referral_offers USING btree (created_at) WHERE (status = 'pending'::text);

CREATE TABLE public.referral_requests (
    id bigint NOT NULL,
    seeker_user_id bigint NOT NULL,
    company_slug text NOT NULL,
    job_id bigint,
    cv_kind text NOT NULL,
    cv_id bigint,
    contact_telegram text,
    contact_email text,
    note text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'sent'::text NOT NULL,
    acted_by bigint,
    acted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT referral_requests_status_check CHECK ((status = ANY (ARRAY['sent'::text, 'contacted'::text, 'declined'::text]))),
    -- An original attachment carries no cv_id; a built one references a cvs row at
    -- creation. cv_id is NOT pinned NOT NULL for 'built' because the FK is ON DELETE
    -- SET NULL — deleting the tailored CV later nulls cv_id while cv_kind stays 'built',
    -- and that must not fail the delete. The "built requires a cv_id at creation"
    -- invariant is enforced in the domain layer, not here.
    CONSTRAINT referral_requests_cv_kind_check CHECK (
        ((cv_kind = 'original'::text) AND (cv_id IS NULL)) OR
        (cv_kind = 'built'::text)
    ),
    CONSTRAINT referral_requests_contact_check CHECK ((contact_telegram IS NOT NULL) OR (contact_email IS NOT NULL))
);

ALTER TABLE public.referral_requests ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.referral_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_seeker_user_id_fkey FOREIGN KEY (seeker_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_company_slug_fkey FOREIGN KEY (company_slug) REFERENCES public.companies(slug) ON DELETE CASCADE;

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_cv_id_fkey FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.referral_requests
    ADD CONSTRAINT referral_requests_acted_by_fkey FOREIGN KEY (acted_by) REFERENCES public.users(id) ON DELETE SET NULL;

-- At most one active request per (seeker, company); resolved requests free a re-request.
CREATE UNIQUE INDEX referral_requests_active_seeker_company_idx ON public.referral_requests USING btree (seeker_user_id, company_slug) WHERE (status = 'sent'::text);

-- The seeker's "my requests" list, newest first; also serves the per-day cap count.
CREATE INDEX referral_requests_seeker_created_at_idx ON public.referral_requests USING btree (seeker_user_id, created_at DESC);

-- The referrer inbox: open requests by company (joined to their approved offers).
CREATE INDEX referral_requests_company_sent_idx ON public.referral_requests USING btree (company_slug) WHERE (status = 'sent'::text);
