ALTER TABLE public.direct_applications
    ADD COLUMN viewed_at timestamptz,
    ADD COLUMN employer_notes text NOT NULL DEFAULT '';

CREATE TABLE public.employer_activity (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_slug text NOT NULL REFERENCES public.company_accounts (company_slug) ON DELETE CASCADE,
    actor_id bigint REFERENCES public.users (id) ON DELETE SET NULL,
    application_id bigint REFERENCES public.direct_applications (id) ON DELETE CASCADE,
    action text NOT NULL CHECK (action IN ('stage_changed', 'notes_changed', 'invited', 'invitation_withdrawn', 'member_joined', 'member_removed')),
    previous_stage text,
    stage text,
    subject_email text,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX employer_activity_company_idx ON public.employer_activity (company_slug, id DESC);
CREATE INDEX employer_activity_actor_idx ON public.employer_activity (actor_id) WHERE actor_id IS NOT NULL;
CREATE INDEX employer_activity_application_idx ON public.employer_activity (application_id) WHERE application_id IS NOT NULL;

-- The shared literal predicate for the page and its matching total.
CREATE FUNCTION public.employer_application_matches(needle text, email text, note text, answers jsonb)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT needle = ''
        OR strpos(lower(email), lower(needle)) > 0
        OR strpos(lower(note), lower(needle)) > 0
        OR EXISTS (
            SELECT 1 FROM jsonb_array_elements(answers) AS answer
            WHERE strpos(lower(answer->>'answer'), lower(needle)) > 0
               OR strpos(lower(answer->>'label'), lower(needle)) > 0
        )
$$;

-- Keep employer_team's existing semantics for its other callers.
CREATE FUNCTION public.active_employer_team(actor bigint) RETURNS SETOF bigint
LANGUAGE sql STABLE ROWS 3 AS $$
    SELECT public.employer_team(actor)
    WHERE EXISTS (
        SELECT 1 FROM public.company_accounts ca
        WHERE ca.user_id = actor AND ca.status = 'active'
        UNION ALL
        SELECT 1 FROM public.company_account_members m
        JOIN public.company_accounts ca ON ca.company_slug = m.company_slug
        WHERE m.user_id = actor AND ca.status = 'active'
    )
$$;
