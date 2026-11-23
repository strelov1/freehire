-- Page views come from the existing job_daily_views rollup.
CREATE TABLE public.employer_vacancy_apply_clicks (
    job_id bigint NOT NULL REFERENCES public.jobs (id) ON DELETE CASCADE,
    day date NOT NULL,
    clicks bigint NOT NULL,
    PRIMARY KEY (job_id, day)
);
