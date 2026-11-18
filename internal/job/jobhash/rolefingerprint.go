package jobhash

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
)

// NormalizedRoleTitle returns the title in the form RoleFingerprint hashes (visible
// text, lowercased, whitespace-collapsed, trailing clause stripped), without the
// company slug or description. Cross-company aggregation (internal/job/recentfeed)
// needs this; RoleFingerprint is scoped to one company.
func NormalizedRoleTitle(title string) string {
	return normalizeRoleText(stripTrailingClause(title))
}

// RoleFingerprint returns a deterministic hex fingerprint of a job's role identity:
// company, normalized title and normalized description. Volatile fields (posted_at,
// url, public_slug, source, external_id, location) are excluded, so a repost under a
// new external_id resolves to the same fingerprint.
//
// This is the opposite of Of: Of is the change signal (includes posted_at), this is
// the identity signal. Never use content_hash to cluster reposts.
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

// htmlTag matches a single HTML tag. Descriptions are stored as sanitized HTML
// (htmltext.Sanitize), so there is no stray "<"/">" in visible text to over-strip.
var htmlTag = regexp.MustCompile(`<[^>]*>`)

// normalizeRoleText reduces s to visible text (tags removed, entities decoded),
// lower-cases and collapses whitespace, so markup, entity or case differences still
// cluster. Tags become a space so "a</p><p>b" keeps its word boundary. The tag strip
// runs before entity decoding so an escaped "a &lt; b" is never mistaken for a tag.
// No stemming or punctuation stripping, to avoid over-merging distinct roles.
func normalizeRoleText(s string) string {
	s = htmlTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// trailingClause matches the last clause of a title: a comma, or a space-delimited
// pipe/at/dash, followed by a separator-free tail anchored to the end. RE2's leftmost
// match lands on the last separator, so only one clause is removed. Dash/pipe/at need
// a leading space so an in-word hyphen (front-end) is never a separator.
var trailingClause = regexp.MustCompile(`(\s*,\s*|\s+[|@]\s*|\s+[-–—]\s*)[^,|@\-–—]*$`)

// stripTrailingClause removes a trailing location/qualifier clause ("Senior
// Engineer, Krakau" -> "Senior Engineer") so per-city variants share a fingerprint.
// It leaves the title unchanged when stripping would drop it below two words, so a
// generic single token cannot become a cluster key.
func stripTrailingClause(title string) string {
	stripped := trailingClause.ReplaceAllString(title, "")
	if len(strings.Fields(stripped)) < 2 {
		return title
	}
	return stripped
}
