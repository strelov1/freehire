package cv

// Server-side half of the autopilot run: what may be persisted, and the owner-scoped writes
// that persist it. The wire shape it validates lives in autopilot.go, which is generated to
// TypeScript — keeping the two apart is why the client never sees this file's rules.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	// maxAutopilotEntries bounds one report. A vacancy's requirement list is a dozen or
	// two; a report far past that is a model looping, and the whole thing is replayed
	// into its context on every later turn of the session.
	maxAutopilotEntries = 40
	// maxAutopilotRequirement and maxAutopilotNote bound the text of one entry. Both are
	// display strings — a requirement copied from the analysis, a one-line note.
	maxAutopilotRequirement = 200
	maxAutopilotNote        = 300
)

// SanitizeAutopilotReport validates a report before it is persisted: every status comes
// from the fixed vocabulary, every entry names a requirement, and text is bounded.
//
// Text is trimmed and truncated silently — those are display concerns. A wrong status or a
// nameless requirement is refused, because both would persist something the panel cannot
// show, and the refusal tells the model exactly what to send instead.
func SanitizeAutopilotReport(entries []AutopilotEntry) ([]AutopilotEntry, error) {
	if len(entries) == 0 {
		return nil, errors.New("a run report needs at least one requirement — report every requirement " +
			"you considered, including the ones you could not close")
	}
	if len(entries) > maxAutopilotEntries {
		return nil, fmt.Errorf("a run report holds at most %d requirements, got %d — report the vacancy's "+
			"requirements, not every sentence of the posting", maxAutopilotEntries, len(entries))
	}

	out := make([]AutopilotEntry, 0, len(entries))
	for i, e := range entries {
		clean, err := sanitizeAutopilotEntry(e)
		if err != nil {
			return nil, fmt.Errorf("entry %d %w", i, err)
		}
		out = append(out, clean)
	}
	return out, nil
}

// sanitizeAutopilotEntry validates and clips ONE entry — the rule both SanitizeAutopilotReport
// (a whole report) and MergeAutopilotEntry (the one entry it is about to fold in) apply before
// anything reaches the database.
func sanitizeAutopilotEntry(e AutopilotEntry) (AutopilotEntry, error) {
	requirement := strings.TrimSpace(e.Requirement)
	if requirement == "" {
		return AutopilotEntry{}, errors.New("names no requirement — copy the requirement text from cv_context")
	}
	if !validAutopilotStatus(e.Status) {
		return AutopilotEntry{}, fmt.Errorf("has status %q; valid statuses are %s",
			e.Status, strings.Join(AutopilotStatuses, ", "))
	}
	return AutopilotEntry{
		Requirement: clip(requirement, maxAutopilotRequirement),
		Status:      e.Status,
		Note:        clip(e.Note, maxAutopilotNote),
	}, nil
}

func validAutopilotStatus(s AutopilotStatus) bool {
	switch s {
	case AutopilotClosedBank, AutopilotClosedCandidate, AutopilotOpen, AutopilotNotReached:
		return true
	}
	return false
}

// SetAutopilotReport replaces the run report on an owned CV, or returns ErrNotFound.
// The whole report is written every time: a requirement closed later from the candidate's
// own words arrives as the same list with one entry changed, so there is one write path
// rather than two.
func (s *Store) SetAutopilotReport(ctx context.Context, id uuid.UUID, userID int64, entries []AutopilotEntry) error {
	clean, err := SanitizeAutopilotReport(entries)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	n, err := s.repo.SetAutopilotReport(ctx, id, userID, blob)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MergeAutopilotEntry writes one requirement's outcome into an owned CV's run report,
// replacing the matching entry (requirement text compared case- and whitespace-insensitively)
// or appending a new one when the report holds nothing for it yet.
//
// This is what lets cv_edit report the requirement it just closed in the SAME call as the
// edit, rather than depending on a separate tailor_report call the model may not remember
// to make once the requirement is no longer the one it is thinking about.
//
// The merge itself runs as a single statement on the repository (MergeCVAutopilotEntry),
// not as a Get here followed by a SetAutopilotReport: reading the report and writing it back
// as two separate calls leaves a gap for another merge to land in between, and whichever call
// commits last would overwrite the whole column with its own stale view — silently dropping
// the other call's entry rather than just delaying it.
func (s *Store) MergeAutopilotEntry(ctx context.Context, id uuid.UUID, userID int64, entry AutopilotEntry) error {
	clean, err := sanitizeAutopilotEntry(entry)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	n, err := s.repo.MergeAutopilotEntry(ctx, id, userID, blob)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// decodeAutopilotReport reads the stored report, tolerating an absent one. A stored report
// that will not parse is treated as absent rather than failing the CV read: the panel losing
// a run log must not cost the candidate their CV.
func decodeAutopilotReport(blob []byte) []AutopilotEntry {
	if len(blob) == 0 {
		return nil
	}
	var entries []AutopilotEntry
	if err := json.Unmarshal(blob, &entries); err != nil {
		return nil
	}
	return entries
}
