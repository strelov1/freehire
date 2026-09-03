-- The triage inbox for a crowdsourced link that is not yet a board: a raw URL nobody
-- has classified into (provider, board). This is the review-status slice of the old
-- link_contributions table. Deliberately has no provider/board column — a row here is
-- not a board with missing fields, it is not a board at all yet. Triage deletes the row
-- and inserts the resolved identity into `boards` instead.
CREATE TABLE board_submissions (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url          text NOT NULL,
    submitted_by bigint NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    surface      text NOT NULL DEFAULT 'unknown'
                     CHECK (surface IN ('web', 'telegram', 'discord', 'extension', 'cli', 'unknown')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX board_submissions_url_key ON board_submissions (url);

-- Every users FK needs an index, or deleting an account scans the whole table.
CREATE INDEX board_submissions_submitted_by_idx ON board_submissions (submitted_by);

COMMENT ON TABLE board_submissions IS
    'Unclassified-URL triage inbox (the link_contributions "review" case). A row is '
    'deleted once triage resolves its (provider, board) and inserts into boards.';
