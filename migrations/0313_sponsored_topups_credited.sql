-- A top-up paid for a campaign that was already rejected or ended is recorded but NOT added to
-- the budget (it can never serve), and is refunded. `credited` tells that case apart from an
-- ordinary top-up on a campaign that ended later, whose unused budget stays recorded.
ALTER TABLE sponsored_topups ADD COLUMN credited boolean NOT NULL DEFAULT true;
