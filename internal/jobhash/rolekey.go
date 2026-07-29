package jobhash

// RoleKey returns a CROSS-SOURCE role identity: the company slug and the
// normalized, trailing-clause-stripped title, joined. Two postings of one role
// share a key however differently their descriptions read.
//
// It is deliberately weaker than RoleFingerprint, which also hashes the
// description. That strength is right for clustering reposts WITHIN a source,
// where the same adapter produced both texts. Across sources it is wrong:
// aggregators truncate and rewrite descriptions, so a fingerprint match between an
// aggregator listing and the employer's own board would be a coincidence rather
// than a rule — the cross-check would find nothing in common and report every
// posting as missing from its own company's board.
//
// What survives rewriting is the company and the title, so that is the key. It
// reuses the same normalization RoleFingerprint applies to a title, so a per-city
// variant collapses onto its base role here exactly as it does there.
//
// A title that normalizes to nothing yields the empty string, and a caller MUST
// treat that as "no key" rather than as a key — every blank title would otherwise
// match every other one.
func RoleKey(companySlug, title string) string {
	normalized := normalizeRoleText(stripTrailingClause(title))
	if normalized == "" {
		return ""
	}
	return companySlug + "\x1e" + normalized
}
