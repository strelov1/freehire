package jobhash

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
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

// htmlTag matches a single HTML tag. Descriptions are stored as sanitized HTML (an
// ingest-time bluemonday prose allowlist, see internal/sources.SanitizeHTML), so tags
// are well-formed and their text is entity-escaped — there is no stray "<"/">" in
// visible text for this to over-strip.
var htmlTag = regexp.MustCompile(`<[^>]*>`)

// normalizeRoleText reduces s to its visible text — HTML tags removed and HTML entities
// decoded — then lower-cases and collapses runs of whitespace, so a re-post whose only
// difference is markup, entity encoding, or cosmetic case/spacing still clusters to one
// role. Tags are replaced with a space (not deleted) so words separated only by a block
// element ("a</p><p>b") keep their boundary. The tag strip runs before entity decoding,
// so an escaped angle bracket in visible text ("a &lt; b") is decoded only after real
// tags are gone and is never mistaken for a tag. The normalization stays narrow beyond
// this (no stemming/punctuation stripping) to avoid over-merging distinct roles.
func normalizeRoleText(s string) string {
	s = htmlTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
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
