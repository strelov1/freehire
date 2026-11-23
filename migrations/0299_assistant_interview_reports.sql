CREATE TABLE assistant_interview_reports (
    session_id uuid PRIMARY KEY REFERENCES assistant_sessions(id) ON DELETE CASCADE,
    through_seq integer NOT NULL,
    report jsonb NOT NULL
);
