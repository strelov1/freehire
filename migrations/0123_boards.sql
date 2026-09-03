-- The board catalog: which company crawls on which ATS, under what board id, and
-- whether that board is proven to work. Replaces sources/*.yml (git) as what cmd/ingest
-- reads, and absorbs the recognized (non-review) half of link_contributions' lifecycle.
-- See docs/superpowers/specs/2026-09-03-board-catalog-in-db-design.md.
--
-- boards_identity_key covers only 'pending'/'active' — not 'rejected' or 'retired' — so
-- neither a retired board nor a previously-rejected one permanently occupies its
-- identity: a corrected resubmission after a validation failure is never blocked by its
-- own earlier attempt.
CREATE TABLE boards (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider        text NOT NULL,
    board           text NOT NULL,
    region          text NOT NULL DEFAULT '',
    company         text NOT NULL,
    hub             boolean NOT NULL DEFAULT false,
    tenants         jsonb NOT NULL DEFAULT '{}'::jsonb,
    url             text,
    status          text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'active', 'rejected', 'retired')),
    submitted_by    bigint REFERENCES public.users(id) ON DELETE SET NULL,
    -- Mirrors internal/ingest/contribution's surface vocabulary (web/telegram/discord/
    -- extension/cli/unknown) plus 'curator' for a row cmd/add-board inserted directly.
    surface         text NOT NULL DEFAULT 'curator'
                        CHECK (surface IN ('web', 'telegram', 'discord', 'extension', 'cli',
                                            'unknown', 'curator')),
    rejected_reason text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    activated_at    timestamptz
);

CREATE UNIQUE INDEX boards_identity_key
    ON boards (provider, lower(board), region)
    WHERE status IN ('pending', 'active');

-- Board lookup by provider for cmd/ingest's per-provider crawl query.
CREATE INDEX boards_provider_status_idx ON boards (provider, status);

-- Every users FK needs an index, or deleting an account scans the whole table.
CREATE INDEX boards_submitted_by_idx ON boards (submitted_by);

COMMENT ON TABLE boards IS
    'Board catalog (replaces sources/*.yml). status: pending (unproven, still crawled) '
    '-> active (first crawl succeeded); or rejected (failed insert-time validation) / '
    'retired (curator-removed). submitted_by/surface/url are set for a crowdsourced row '
    'and NULL/"curator" for one added by cmd/add-board.';
