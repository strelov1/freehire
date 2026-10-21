package job

import "strings"

// EmployerSource is the source of every vacancy a verified employer account authors
// (internal/ingest/employer).
const EmployerSource = "employer"

// DirectApplyPrefix leads the external id of an employer vacancy that takes applications
// on freehire rather than linking out. Such a vacancy has no URL to key on, so its
// external id is minted as this prefix plus a UUID — and because the prefix is written in
// the same statement as the row, the apply mode can never disagree with the vacancy or
// change after it (see openspec/changes/add-in-app-apply/design.md, decision 1).
const DirectApplyPrefix = "freehire:"

// IsDirectApply reports whether the posting at (source, externalID) accepts applications
// on freehire. The one predicate every reader asks.
func IsDirectApply(source, externalID string) bool {
	return source == EmployerSource &&
		len(externalID) > len(DirectApplyPrefix) &&
		strings.HasPrefix(externalID, DirectApplyPrefix)
}
