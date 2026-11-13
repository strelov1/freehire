-- migrate: no-transaction
-- Release A: restartable orphan cleanup and online foreign-key validation.
DELETE FROM job_activity a WHERE NOT EXISTS (SELECT 1 FROM jobs j WHERE j.id = a.job_id);
-- VALIDATE is safe to repeat after a partial run, including an already validated constraint.
-- squawk-ignore prefer-robust-stmts
ALTER TABLE job_activity VALIDATE CONSTRAINT job_activity_job_id_fkey;
