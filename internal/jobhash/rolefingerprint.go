package jobhash

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/db"
)

// RoleFingerprint returns a deterministic hex fingerprint of a job's ROLE IDENTITY —
// company, normalized title, and normalized description — deliberately excluding
// every volatile field (posted_at, url, public_slug, source, external_id, location).
// A role reposted under a new external_id with a refreshed posted date therefore
// resolves to the same fingerprint, so the reality signal can cluster reposts.
//
// This is the opposite of Of: Of is the CHANGE signal (it includes posted_at, so a
// repost with a bumped date is "changed" and re-indexed); RoleFingerprint is the
// IDENTITY signal (it ignores posted_at, so reposts collapse to one role). Never use
// content_hash to cluster reposts.
func RoleFingerprint(p db.UpsertJobParams) string {
	const rs = "\x1e"
	var b strings.Builder
	b.WriteString(p.CompanySlug)
	b.WriteString(rs)
	b.WriteString(normalizeRoleText(stripTrailingClause(p.Title)))
	b.WriteString(rs)
	b.WriteString(normalizeRoleText(p.Description))

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// normalizeRoleText lower-cases and collapses runs of whitespace so cosmetic case or
// spacing differences in a re-post do not split one role. The normalization stays
// narrow (no stemming/punctuation stripping) to avoid over-merging distinct roles.
func normalizeRoleText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// trailingClause matches the last clause of a title: a separator — a comma, or a
// space-delimited pipe/at/dash (`-`, en-dash, em-dash) — followed by a final segment
// that contains no further separator, anchored to the end. RE2's leftmost match lands
// on the LAST separator (an earlier one cannot reach `$` with a separator-free tail),
// so only one trailing clause is removed. The dash/pipe/at require a leading space so
// an in-word hyphen (front-end) is never a separator; a comma needs none.
var trailingClause = regexp.MustCompile(`(\s*,\s*|\s+[|@]\s*|\s+[-–—]\s*)[^,|@\-–—]*$`)

// stripTrailingClause removes a trailing location/qualifier clause from a job title
// (e.g. "Senior Engineer, Krakau" -> "Senior Engineer") so per-city variants of one
// role share a fingerprint. It strips only a suffix — a leading grade like "Senior"
// is never touched — and leaves the title unchanged when stripping would drop it below
// two words, so a too-generic single token cannot become a cluster key.
func stripTrailingClause(title string) string {
	stripped := trailingClause.ReplaceAllString(title, "")
	if len(strings.Fields(stripped)) < 2 {
		return title
	}
	return stripped
}
