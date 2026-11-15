-- Apply before deploying evidence freshness reads.
ALTER TABLE cover_letters ADD COLUMN evidence_revision bigint NOT NULL DEFAULT 0,
 ADD COLUMN evidence_fingerprint text NOT NULL DEFAULT '';
