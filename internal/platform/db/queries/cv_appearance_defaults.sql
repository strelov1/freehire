-- name: GetCVAppearanceDefaults :one
-- The user's saved CV appearance defaults, if any. No row means the user has never saved
-- any — the caller falls back to the system defaults rather than treating this as an error.
SELECT user_id, template_id, style, margins, updated_at
FROM cv_appearance_defaults
WHERE user_id = $1;

-- name: UpsertCVAppearanceDefaults :one
-- Replaces the user's CV appearance defaults wholesale — there is exactly one row per user,
-- so a save always means "this is the new complete set", never a partial patch.
INSERT INTO cv_appearance_defaults (user_id, template_id, style, margins, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (user_id) DO UPDATE
SET template_id = EXCLUDED.template_id,
    style = EXCLUDED.style,
    margins = EXCLUDED.margins,
    updated_at = now()
RETURNING user_id, template_id, style, margins, updated_at;
