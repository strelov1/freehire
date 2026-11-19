-- Recruiter contacts found for a job, cached so a second viewer costs no provider lookup.
-- One row per job; `contacts` is the list exactly as served (name, title, email, confidence),
-- `[]` when nothing usable was found. The provider is not recorded here: it is known only to
-- the contact-finder service. Rows are personal data of third parties, so
-- cmd/sweep-recruiter-contacts deletes them after 30 days (7 for an empty result).
CREATE TABLE public.job_recruiter_contacts (
    job_id bigint PRIMARY KEY REFERENCES public.jobs (id) ON DELETE CASCADE,
    contacts jsonb NOT NULL,
    found_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX job_recruiter_contacts_found_at_idx ON public.job_recruiter_contacts (found_at);
