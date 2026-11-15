-- Apply before deploying the binary that reads cover-letter evidence selections.
CREATE TABLE cover_letter_evidence (
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 job_id bigint NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
 revision bigint NOT NULL CHECK (revision > 0),
 atom_ids uuid[] NOT NULL DEFAULT '{}',
 PRIMARY KEY (user_id,job_id),
 CHECK (cardinality(atom_ids) <= 50)
);
