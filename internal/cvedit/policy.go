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
// This replaces access control by omission. The named-op vocabulary it succeeds kept the
// agent away from the candidate's contact details by having no operation that could name
// them — a real defence, but an accidental one that widens silently every time the
// vocabulary grows. Stated as a table, it can be read, tested, and argued with.
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
				return fmt.Errorf("%w: %q is the candidate's own. Edit the body of the CV instead — "+
					"experience, summary, skills, projects and education are yours to rewrite", ErrForbiddenPath, op.Path)
			}
		}
	}
	return nil
}

// pathTouches reports whether writing at `p` writes the named place — which is true in BOTH
// directions, and getting that wrong was a hole.
//
// Downwards is obvious: denying `header.links` must deny `header.links[0]`. Upwards is the one
// that bites: `header` is not inside `header.email`, but writing a whole `header` object
// replaces every contact identifier in it at once. The addressable vocabulary published to the
// model offers the container alongside the leaf, so a denial that only looks downwards is a
// denial the model can step over.
func pathTouches(p Path, place string) bool {
	s := string(p)
	return s == place || nests(s, place) || nests(place, s)
}

// nests reports whether `inner` sits inside `outer` — the same field or index boundary check
// both directions of pathTouches need, and what keeps `experience[1]` from matching
// `experience[10]`.
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
// The list is inverted on purpose, and the earlier version — a list of the places that DO
// carry a claim — is why. It named summary, bullets, stack and skills, so a degree nobody
// earned, a certification nobody holds and a job title nobody had all landed uncited. They
// are the larger lie: a recruiter checks a diploma, not a bullet's phrasing.
//
// Listing what is exempt makes the default safe. A field added to the document is a claim
// until someone says otherwise, which is the direction this has to fail in.
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
	// The header is the candidate's identity rather than a claim about their career, and the
	// agent is denied most of it by policy anyway. What it may still write — the location —
	// is the candidate's own to state and is not evidenced in the bank.
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
// writes the bullets inside it, and writing `education` replaces the whole section. A write to
// a container of presentation — `style` — is exempt only because everything inside it is.
func assertsAClaim(op Op) bool {
	if op.Kind != OpSet && op.Kind != OpInsert {
		return false
	}
	return !presentationShapes[shapeOf(op.Path)]
}

// requireEvidence holds the rule the whole tailoring capability exists to keep: a sentence
// about what the candidate did cannot reach the page unless it traces to something they
// asserted. It runs in the service path rather than in a system prompt, because a rule that
// lives only in a prompt is one a long conversation eventually loses.
//
// Each writing operation answers for itself and one uncited operation refuses the whole
// batch — otherwise a claim could ride in among valid ones.
func (e *Editor) requireEvidence(ctx context.Context, userID int64, ch Change) error {
	if e.gate == nil {
		return nil
	}
	// Only the agent has to cite. The rule is that a MODEL's inference must not reach the
	// page — the candidate writing about their own career is the source the bank exists to
	// record, and asking them for a citation would make their own editor unusable.
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
