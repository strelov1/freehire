-- A person's list of companies they want to hear about: the alert for it is a saved search
-- carrying `watchlist=true`, resolved against this table when it is run.
--
-- company_slug has NO foreign key to companies, for the reason company_slug_aliases has none:
-- companies is derived from jobs and DeleteOrphanCompanies removes a row once no job names
-- it, so a cascade would delete a person's entry the day an employer's last posting went —
-- and an employer with nothing open is exactly the one a watchlist is for. A slug a merge
-- retires is followed through company_slug_aliases when the list is read.
--
-- The primary key leads with user_id, which is also the index Postgres does not create for
-- the users reference.
CREATE TABLE company_watchlist (
    user_id      bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    company_slug text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_slug)
);
