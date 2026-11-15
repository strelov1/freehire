package job

import "strings"

// EmployerSource is the source of every vacancy a verified employer account authors
// (internal/ingest/employer).
const EmployerSource = "employer"

// DirectApplyPrefix leads the external id of an employer vacancy that takes applications on
// freehire rather than linking out. With no URL to key on, the id is this prefix plus a
// UUID, written in the same statement as the row, so the apply mode never changes.
const DirectApplyPrefix = "freehire:"

// IsDirectApply reports whether the posting at (source, externalID) accepts applications
// on freehire. The one predicate every reader asks.
func IsDirectApply(source, externalID string) bool {
	return source == EmployerSource &&
		len(externalID) > len(DirectApplyPrefix) &&
		strings.HasPrefix(externalID, DirectApplyPrefix)
}
