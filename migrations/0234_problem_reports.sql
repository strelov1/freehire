-- Apply before deploying the report_problem MCP tool.
CREATE TABLE problem_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    category text NOT NULL CHECK (category IN ('tool_error', 'missing_capability', 'other')),
    goal text NOT NULL CHECK (char_length(goal) BETWEEN 1 AND 500),
    expected text NOT NULL CHECK (char_length(expected) BETWEEN 1 AND 1000),
    actual text NOT NULL CHECK (char_length(actual) BETWEEN 1 AND 2000),
    tool_name text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    fingerprint text NOT NULL,
    request_id text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed')),
    -- squawk-ignore prefer-bigint-over-int -- daily submissions are bounded at 20
    request_count integer NOT NULL DEFAULT 1 CHECK (request_count > 0),
    first_reported_at timestamptz NOT NULL DEFAULT now(),
    last_reported_at timestamptz NOT NULL DEFAULT now(),
    resolution_note text NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX problem_reports_open_key ON problem_reports (user_id, fingerprint) WHERE status = 'open';
CREATE INDEX problem_reports_user_id_idx ON problem_reports (user_id);
CREATE INDEX problem_reports_triage_idx ON problem_reports (status, last_reported_at);

CREATE TABLE problem_report_daily (
    user_id bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    day date NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- capped at 20 per account per UTC day
    writes integer NOT NULL CHECK (writes BETWEEN 1 AND 20),
    PRIMARY KEY (user_id, day)
);
