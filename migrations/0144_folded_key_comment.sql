-- Say what folded_key actually holds.
--
-- 0112 described it as "alias_slug with hyphens removed", and until the curated alias list
-- landed that was a true-enough paraphrase: every alias in a folded group reached the canon
-- through the group's shared fold, and for those rows the two readings agree.
--
-- They are not the same reading. The writer computes normalize.CompanyKey of the alias's
-- NAME, and CompanyKey strips trailing legal forms before folding — so a company stored as
-- `sun-technologies-inc` and named "Sun Technologies, Inc." keys at `suntechnologies`, not at
-- `suntechnologiesinc`. The name is the correct side: ingest folds the name the SOURCE sends,
-- not a slug the catalogue derived, so a key built from the slug would be a key no crawl ever
-- asks for.
--
-- What made the distinction load-bearing is the curated list (cmd/merge-companies/curated.go).
-- Its groups are, by definition, spellings that do NOT share a fold — "Exadel" beside "Exadel
-- Inc (Website)" — so each alias records its own key rather than the group's. A reader who
-- believed the old comment would look at those rows, see keys that differ inside one group,
-- and take a correct table for a corrupt one.
--
-- Comment-only. The column, its index, and every stored row are untouched; nothing here takes
-- a lock worth naming.
COMMENT ON COLUMN company_slug_aliases.folded_key IS
    'normalize.CompanyKey of the alias company NAME — which strips a trailing legal form '
    'before folding, so it is NOT simply alias_slug with the hyphens removed. Ingest folds '
    'the name its source sends and looks the result up here, which is how a spelling never '
    'merged before still resolves to the canon its folded form already owns. Rows written by '
    'a CURATED merge deliberately carry keys that differ within one group: such a group is '
    'defined by its members NOT sharing a fold.';
