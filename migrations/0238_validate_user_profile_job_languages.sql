-- Validate separately so the scan does not retain the ADD COLUMN lock.
ALTER TABLE user_profiles VALIDATE CONSTRAINT user_profiles_job_languages_cap;
