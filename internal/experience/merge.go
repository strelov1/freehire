package experience

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Merge refuse reasons. Worded for the model: the assistant tool returns them
// verbatim, and that message is the only route to self-correction within a turn.
var (
	// ErrInvalidMerge is a pair that is not exactly two distinct owned atoms
	// (same id twice, empty list, etc.).
	ErrInvalidMerge = errors.New("experience: merge needs exactly two different atom ids")
	// ErrMergeCrossEmployment is a pair attached to two different employments
	// (including one placed and one unplaced).
	ErrMergeCrossEmployment = errors.New("experience: cannot merge atoms from different roles — attach them to the same role first, or merge two unplaced atoms")
	// ErrContextRequired is an interactive create with empty context while the
	// owner has opted into requiring situation paragraphs. Checked in the
	// handler/tool, not in Store.AddAtom — import must stay ungated.
	ErrContextRequired = errors.New("experience: context is required for new achievements — add a short situation paragraph, or ask the interviewer to turn the requirement off")
)

// mergeCandidate is one side of a merge, carrying the created_at used for
// keep-selection ties. The domain Atom does not expose CreatedAt on the wire.
type mergeCandidate struct {
	Atom
	CreatedAt time.Time
}

// chooseKeep returns which of a, b to keep (true → a) by richness score, then
// older created_at, then smaller id.
func chooseKeep(a, b mergeCandidate) bool {
	sa, sb := richnessScore(a.Atom), richnessScore(b.Atom)
	if sa != sb {
		return sa > sb
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID.String() < b.ID.String()
}

func richnessScore(a Atom) int {
	score := len(a.Metrics) + len(a.Skills)
	if strings.TrimSpace(a.Context) != "" {
		score++
	}
	if a.Provenance.Publishable() {
		score++
	}
	return score
}

// unionForMerge builds the kept atom's post-merge fields. Claim, employment,
// and source_ref stay on keep. Context is the longer non-empty string; metrics
// and skills are unioned (keep first) then Sanitized.
//
// Provenance stays keep's own — never lose's, even when lose is publishable and keep is
// not. The merged Claim is keep.Claim verbatim: if that text was never candidate-asserted,
// tagging the merge as publishable because the DISCARDED atom happened to be would let an
// agent-inferred, unconfirmed claim reach the CV evidence gate as if the candidate had said
// it (see internal/handler/AGENTS.md — the provenance check lives here, not in a prompt).
func unionForMerge(keep, lose Atom) Atom {
	out := keep
	out.Context = richerContext(keep.Context, lose.Context)
	out.Metrics = unionStrings(keep.Metrics, lose.Metrics)
	out.Skills = unionStrings(keep.Skills, lose.Skills)
	out.Sanitize()
	return out
}

func richerContext(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a != "" && b != "":
		if utf8.RuneCountInString(b) > utf8.RuneCountInString(a) {
			return b
		}
		return a
	case b != "":
		return b
	default:
		return a
	}
}

func unionStrings(keep, lose []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, keep...), lose...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// sameEmploymentBucket reports whether two atoms may be merged: both unplaced
// or the same employment id.
func sameEmploymentBucket(a, b Atom) bool {
	if a.EmploymentID == nil && b.EmploymentID == nil {
		return true
	}
	if a.EmploymentID == nil || b.EmploymentID == nil {
		return false
	}
	return *a.EmploymentID == *b.EmploymentID
}

// validateMergePair checks the pair rules before any write. Returns a sentinel
// the handler maps to 4xx; never persists.
func validateMergePair(a, b uuid.UUID, left, right Atom) error {
	if a == uuid.Nil || b == uuid.Nil || a == b {
		return ErrInvalidMerge
	}
	if !sameEmploymentBucket(left, right) {
		return ErrMergeCrossEmployment
	}
	return nil
}
