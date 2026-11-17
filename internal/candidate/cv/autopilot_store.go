package cv

// Server-side half of the autopilot run: what may be persisted, and the owner-scoped writes
// that persist it. The wire shape it validates lives in autopilot.go (generated to TypeScript).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/platform/llm"
)

const (
	// maxAutopilotEntries bounds one report; far past a dozen or two requirements is a model
	// looping, and the report is replayed into its context on every later turn.
	maxAutopilotEntries = 40
	// maxAutopilotRequirement and maxAutopilotNote bound the display text of one entry.
	maxAutopilotRequirement = 200
	maxAutopilotNote        = 300
)

// SanitizeAutopilotReport validates a report before it is persisted: every status comes
// from the fixed vocabulary, every entry names a requirement, and text is bounded.
//
// Text is trimmed and truncated silently. A wrong status or nameless requirement is refused,
// since the panel could not show it; the refusal tells the model what to send.
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

// sanitizeAutopilotEntry validates and clips ONE entry, the rule both SanitizeAutopilotReport
// and MergeAutopilotEntry apply before anything reaches the database.
func sanitizeAutopilotEntry(e AutopilotEntry) (AutopilotEntry, error) {
	requirement := strings.TrimSpace(e.Requirement)
	if requirement == "" {
		return AutopilotEntry{}, errors.New("names no requirement — copy the requirement text from cv_context")
	}
	if !validAutopilotStatus(e.Status) {
		return AutopilotEntry{}, fmt.Errorf("has status %q; valid statuses are %s",
			e.Status, strings.Join(AutopilotStatuses, ", "))
	}
	unverified := llm.TrimTruncateRunes(e.Unverified, maxAutopilotNote)
	if unverified != "" && (e.Status == AutopilotClosedBank || e.Status == AutopilotClosedCandidate) {
		e.Status = AutopilotPartial
	}
	if e.Status == AutopilotPartial && unverified == "" {
		return AutopilotEntry{}, errors.New("has partial evidence but names no unverified part")
	}
	return AutopilotEntry{
		Requirement: llm.TrimTruncateRunes(requirement, maxAutopilotRequirement),
		Status:      e.Status,
		Note:        llm.TrimTruncateRunes(e.Note, maxAutopilotNote),
		Unverified:  unverified,
	}, nil
}

func validAutopilotStatus(s AutopilotStatus) bool {
	switch s {
	case AutopilotClosedBank, AutopilotClosedCandidate, AutopilotPartial, AutopilotOpen, AutopilotNotReached:
		return true
	}
	return false
}

// SetAutopilotReport replaces the run report on an owned CV, or returns ErrNotFound. The
// whole report is written every time, so there is one write path.
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
// or appending a new one. It lets cv_edit report the requirement it just closed in the SAME
// call as the edit.
//
// The merge runs as one statement on the repository (MergeCVAutopilotEntry), not Get then
// SetAutopilotReport: that gap would let the last commit overwrite the column and silently
// drop the other call's entry.
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

// decodeAutopilotReport reads the stored report, tolerating an absent one. One that will not
// parse is treated as absent: losing a run log must not cost the candidate their CV.
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
