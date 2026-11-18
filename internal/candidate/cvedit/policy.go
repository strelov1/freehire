package cvedit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrForbiddenPath is returned when an actor addresses a place it may not change. Handlers
// render it for the candidate as a 403; for a model it is a tool error whose message names
// what it may edit instead.
var ErrForbiddenPath = errors.New("cvedit: that part of the CV is not yours to change")

// ErrEvidenceRequired is returned when an operation writes a claim about the candidate
// without citing what it rests on.
var ErrEvidenceRequired = errors.New("cvedit: writing this needs evidence")

// Policy says what each actor may change.
//
// Access is an explicit table, not omission from the op vocabulary, which would widen
// silently as the vocabulary grows.
type Policy struct {
	denied map[Actor][]string
}

// DefaultPolicy keeps the tailoring agent out of the candidate's identity and away from the
// two columns that are the candidate's own presentation choices. The candidate is denied
// nothing: it is their CV.
func DefaultPolicy() Policy {
	return Policy{denied: map[Actor][]string{
		ActorAgent: {
			"header.full_name",
			"header.email",
			"header.phone",
			"header.links",
			"title",
			"template_id",
		},
	}}
}

// Allows checks a whole batch before any of it is applied, so a refusal is never a partial
// edit. The message names what the actor may change, because for a model an error is its
// only route to correcting itself inside the turn.
func (p Policy) Allows(actor Actor, ops []Op) error {
	denied := p.denied[actor]
	if len(denied) == 0 {
		return nil
	}
	for _, op := range ops {
		for _, place := range denied {
			if pathTouches(op.Path, place) {
				// The agent writes the job's title into `title`; the generic refusal did not
				// stop it because it never said the field is not printed.
				if place == "title" {
					return fmt.Errorf("%w: %q is this CV's name in the candidate's list and is not printed "+
						"on the CV. To aim the CV at the role, rewrite the summary and the experience bullets instead",
						ErrForbiddenPath, op.Path)
				}
				return fmt.Errorf("%w: %q is the candidate's own. Edit the body of the CV instead — "+
					"experience, summary, skills, projects and education are yours to rewrite", ErrForbiddenPath, op.Path)
			}
		}
	}
	return nil
}

// pathTouches reports whether writing at `p` writes the named place, in BOTH directions:
// denying `header.links` must deny `header.links[0]`, and writing a whole `header` replaces
// `header.email` too. The model is offered the container alongside the leaf, so a
// downwards-only denial can be stepped over.
func pathTouches(p Path, place string) bool {
	s := string(p)
	return s == place || nests(s, place) || nests(place, s)
}

// nests reports whether `inner` sits inside `outer`, on a field or index boundary
// (`experience[1]` does not match `experience[10]`).
func nests(inner, outer string) bool {
	if !strings.HasPrefix(inner, outer) || len(inner) == len(outer) {
		return false
	}
	next := inner[len(outer)]
	return next == '.' || next == '['
}

// EvidenceGate answers whether a cited piece of banked evidence exists, belongs to this
// candidate, and may be written into a CV. The experience bank implements it; the editor
// only knows the question.
type EvidenceGate interface {
	Publishable(ctx context.Context, userID int64, evidenceID string) error
}

// presentationShapes are the places that assert NOTHING about the candidate: how the CV
// looks, and what it is called. Everything else in the document is a claim.
//
// The list is inverted on purpose: listing what is exempt makes the default safe, since a new
// document field is a claim until someone says otherwise. (A list of claim-bearing places let
// an unearned degree, certification or job title land uncited.)
var presentationShapes = map[string]bool{
	"title":             true,
	"template_id":       true,
	"style":             true,
	"style.font_family": true,
	"style.font_size":   true,
	"style.line_height": true,
	"margins":           true,
	"margins.top":       true,
	"margins.right":     true,
	"margins.bottom":    true,
	"margins.left":      true,
	// The header is identity, not a career claim; the agent is denied most of it by policy, and
	// the location it may write is not evidenced in the bank.
	"header":           true,
	"header.full_name": true,
	"header.email":     true,
	"header.phone":     true,
	"header.location":  true,
	"header.links":     true,
	"header.links[]":   true,
}

var pathIndex = regexp.MustCompile(`\[\d+\]`)

// shapeOf strips the indices from a path, so one entry's third bullet and another's first
// are recognised as the same kind of place.
func shapeOf(p Path) string { return pathIndex.ReplaceAllString(string(p), "[]") }

// assertsAClaim reports whether an operation puts a claim about the candidate on the page.
// Removing and moving assert nothing: they rearrange or delete what was already said.
//
// Anything that is not presentation is a claim, INCLUDING a container: writing `experience[0]`
// writes the bullets inside it. A presentation container such as `style` is exempt only
// because everything inside it is.
func assertsAClaim(op Op) bool {
	if op.Kind != OpSet && op.Kind != OpInsert {
		return false
	}
	return !presentationShapes[shapeOf(op.Path)]
}

// requireEvidence enforces that a sentence about what the candidate did cannot reach the page
// unless it traces to something they asserted. It lives in the service path, not a system
// prompt that a long conversation could lose.
//
// Each writing operation answers for itself, and one uncited operation refuses the whole batch.
func (e *Editor) requireEvidence(ctx context.Context, userID int64, ch Change) error {
	if e.gate == nil {
		return nil
	}
	// Only the agent cites: the candidate is the source the bank records.
	if ch.Actor != ActorAgent {
		return nil
	}
	for _, op := range ch.Ops {
		if !assertsAClaim(op) {
			continue
		}
		if strings.TrimSpace(op.EvidenceID) == "" {
			return fmt.Errorf("%w: %q states something about the candidate, so it needs evidence_id — "+
				"the id of the banked achievement it rests on, from experience_search. If the bank holds "+
				"nothing on this point, ask the candidate and record their answer with experience_add first",
				ErrEvidenceRequired, op.Path)
		}
		if err := e.gate.Publishable(ctx, userID, strings.TrimSpace(op.EvidenceID)); err != nil {
			return err
		}
	}
	return nil
}
