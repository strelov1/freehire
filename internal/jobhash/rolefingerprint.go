package jobhash

import (
	"crypto/sha256"
	"encoding/hex"
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
	b.WriteString(normalizeRoleText(p.Title))
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
