-- Per-user CV appearance defaults: the template, typography, and margins a candidate wants
-- every NEW CV to start from. One row per user — deliberately not a column on `users`, so
-- the row is optional and drops out cleanly with ON DELETE CASCADE.
--
-- style/margins are jsonb rather than their own columns because they already have a
-- validated, versioned shape in Go (cv.Style / cv.Margins) that the CV document itself
-- stores the same way inside cvs.data; template_id stays its own column to mirror cvs.
--
-- Saving a row here never touches an existing CV — see the add-cv-appearance-defaults change.
CREATE TABLE IF NOT EXISTS cv_appearance_defaults (
    user_id     bigint      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    template_id text        NOT NULL,
    style       jsonb       NOT NULL,
    margins     jsonb       NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
