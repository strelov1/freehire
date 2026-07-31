package cvedit

import (
	"time"

	"github.com/google/uuid"
)

// wireTime is how a timestamp travels: RFC 3339, the shape json.Marshal would produce anyway.
// Stated as a string because the generated TypeScript has no mapping for time.Time and would
// otherwise land as `any`, which is a type the client cannot check.
func wireTime(t time.Time) string { return t.Format(time.RFC3339) }

// This file holds the revision's wire shape — what the history feed shows and what the
// preview highlights. It is generated to TypeScript, so it stays free of server-only
// concerns: the operations themselves, the policy and the repository live elsewhere.

// RevisionView is one entry in the history feed.
//
// It deliberately does NOT carry the operations. The feed needs to say what changed and
// where, not to reproduce it: the addresses are enough to underline the right lines in the
// preview, while the inverses hold the candidate's own earlier text and have no business
// leaving the server for a list view.
type RevisionView struct {
	ID string `json:"id"`
	// Actor is who made the change: the candidate, the agent, or the system.
	Actor string `json:"actor"`
	// Origin is the entry point it arrived through — the editor, the template picker, the
	// tailoring agent, the CLI, an import.
	Origin string `json:"origin"`
	// BatchID groups the edits of one agent turn, so the feed can offer to undo the run as
	// a whole. Empty for a change that stands alone.
	BatchID string `json:"batch_id,omitempty"`
	// Title describes the change in the application's own words. It is generated from the
	// operations on the server and never written by a model.
	Title string `json:"title"`
	// Note is the agent's own reason for the edit, when it gave one. It is the model's text
	// and must be rendered as the agent's words, attributed — never as the entry's own
	// description.
	Note string `json:"note,omitempty"`
	// Paths are the places this revision touched, e.g. `experience[2].bullets[1]`. The
	// preview underlines the nodes they address.
	Paths []string `json:"paths"`
	// Reverted says the change has already been undone, which is what greys out its control
	// rather than removing the entry: the log is never rewritten.
	Reverted bool `json:"reverted"`
	// Undoable says the entry has something to reverse. A milestone — "this CV was created"
	// — has not: undoing it would mean deleting the CV, which is a different action with its
	// own button. Stated rather than inferred from an empty path list, because "changed
	// nothing addressable" and "cannot be undone" are different facts.
	Undoable bool `json:"undoable"`
	// RevertsID names the revision this one undid, when it is itself an undo.
	RevertsID string `json:"reverts_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

// View renders a revision for the feed.
func (r Revision) View() RevisionView {
	out := RevisionView{
		ID:        r.ID.String(),
		Actor:     string(r.Actor),
		Origin:    string(r.Origin),
		Title:     r.Title,
		Note:      r.Note,
		Paths:     r.Paths(),
		Reverted:  r.Reverted(),
		Undoable:  len(r.Inverse) > 0 && !r.Reverted(),
		CreatedAt: wireTime(r.CreatedAt),
	}
	if r.BatchID != uuid.Nil {
		out.BatchID = r.BatchID.String()
	}
	if r.RevertsID != uuid.Nil {
		out.RevertsID = r.RevertsID.String()
	}
	return out
}

// Paths is the distinct set of places a revision touched, in the order it touched them.
// Always a list, never nil: "changed nothing anywhere" and "we did not say" are different
// answers, and the preview treats an absent field as the second.
func (r Revision) Paths() []string {
	seen := make(map[Path]bool, len(r.Ops))
	out := make([]string, 0, len(r.Ops))
	for _, op := range r.Ops {
		if seen[op.Path] {
			continue
		}
		seen[op.Path] = true
		out = append(out, string(op.Path))
	}
	return out
}

// Views renders a feed.
func Views(revisions []Revision) []RevisionView {
	out := make([]RevisionView, 0, len(revisions))
	for _, r := range revisions {
		out = append(out, r.View())
	}
	return out
}
