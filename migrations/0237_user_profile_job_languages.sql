-- Apply before deploying code that reads job_languages.
ALTER TABLE user_profiles
    ADD COLUMN job_languages text[] NOT NULL DEFAULT '{}';

ALTER TABLE user_profiles
    ADD CONSTRAINT user_profiles_job_languages_cap CHECK (cardinality(job_languages) <= 200) NOT VALID;
