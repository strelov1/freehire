CREATE TABLE assistant_interview_reports (
    session_id uuid PRIMARY KEY REFERENCES assistant_sessions(id) ON DELETE CASCADE,
    report jsonb NOT NULL
);
