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
		requirement := strings.TrimSpace(e.Requirement)
		if requirement == "" {
			return nil, fmt.Errorf("entry %d names no requirement — copy the requirement text from cv_context", i)
		}
		if !validAutopilotStatus(e.Status) {
			return nil, fmt.Errorf("entry %d has status %q; valid statuses are %s",
				i, e.Status, strings.Join(AutopilotStatuses, ", "))
		}
		out = append(out, AutopilotEntry{
			Requirement: clip(requirement, maxAutopilotRequirement),
			Status:      e.Status,
			Note:        clip(e.Note, maxAutopilotNote),
		})
	}
	return out, nil
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
func (s *Store) MergeAutopilotEntry(ctx context.Context, id uuid.UUID, userID int64, entry AutopilotEntry) error {
	rec, err := s.Get(ctx, id, userID)
	if err != nil {
		return err
	}
	entries := rec.AutopilotReport
	target := strings.ToLower(strings.TrimSpace(entry.Requirement))
	replaced := false
	for i, e := range entries {
		if strings.ToLower(strings.TrimSpace(e.Requirement)) == target {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return s.SetAutopilotReport(ctx, id, userID, entries)
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
