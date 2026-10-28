-- Where a duplicate-marker pass that walks the catalogue one company at a time stopped,
-- so the next run resumes there instead of starting over (cmd/reindex's fuzzy collapse).
--
-- The pass visits ~383k companies and measured on prod 2026-10-28 it ran out its 4h unit
-- timeout 155k companies in. It walked the same order every run, so the companies past
-- that point were never reached at all — not deduplicated late, never. A run now records
-- the last company it FINISHED as it goes (the process is killed by SIGTERM, so a write
-- only at the end would be lost exactly when it matters), the next run takes the list
-- from the company after it, and a run that reaches the end clears the row's value so
-- the walk starts again from the top.
--
-- One row per pass. after_company is '' for "start from the beginning", which is also
-- what a missing row means.
CREATE TABLE marker_pass_cursors (
    pass          text PRIMARY KEY,
    after_company text NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE marker_pass_cursors IS
    'Resume point of a per-company duplicate-marker pass: the last company a run finished. '
    'Machine-owned by cmd/reindex; empty after_company means start from the top.';
