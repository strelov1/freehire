package experience

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/skilltag"
)

// Scoring weights. The ordering between them is the contract, not the absolute values: a
// canonical skill match must dominate word overlap, and the role's stack must sit between
// the two — evidence whose own text names the technology beats evidence that merely comes
// from a role that used it.
const (
	skillWeight = 10.0 // the atom's own canonical skills match the requirement's
	stackWeight = 3.0  // the atom's ROLE ran on the technology, though the bullet is silent
	wordWeight  = 1.0  // a non-stopword shared with the requirement's text
	currentRole = 2.0  // evidence from the role the candidate still holds
	// inferredFactor keeps an agent's own inference in the results and below everything
	// the candidate asserted. Dropping it entirely would be worse: the agent needs to see
	// its own hypothesis in order to ask the candidate about it.
	inferredFactor = 0.4
)

// stopwords are the words that would otherwise make every atom match every requirement.
// Deliberately short: this is a relevance floor, not a linguistic model, and a word that
// carries meaning in a CV bullet ("led", "built", "team") must never end up here.
var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true,
	"by": true, "for": true, "from": true, "in": true, "is": true, "it": true, "of": true,
	"on": true, "or": true, "our": true, "that": true, "the": true, "their": true,
	"this": true, "to": true, "was": true, "were": true, "will": true, "with": true,
	"you": true, "your": true, "we": true,
}

// Query is what retrieval scores against: a vacancy requirement in words, the canonical
// skills it names, or both. Either half may be empty — a requirement often names no skill
// at all ("led a team of five"), which is precisely why retrieval cannot be a skill filter.
type Query struct {
	Text   string
	Skills []string
}

// Match is one scored piece of evidence, with the place it came from when it has one.
type Match struct {
	Atom       Atom
	Employment *Employment
	Score      float64
}

// Retrieve returns the owner's evidence for a requirement, best first and no more than
// limit entries. It performs no model call: scoring is a linear pass over the owner's
// atoms, which for a bank of tens-to-hundreds is cheaper and far more predictable than an
// embedding pipeline.
//
// Evidence that scores zero is omitted rather than returned as a weak match. A
// zero-scoring "match" reads to an agent as evidence, and evidence is what licenses a CV
// bullet — an unanswered requirement must present itself as the gap it is.
func (s *Store) Retrieve(ctx context.Context, userID int64, q Query, limit int) ([]Match, error) {
	atoms, err := s.ListAtoms(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(atoms) == 0 {
		return nil, nil
	}
	employments, err := s.ListEmployments(ctx, userID)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]*Employment, len(employments))
	for i := range employments {
		byID[employments[i].ID] = &employments[i]
	}

	q.Skills = skilltag.Canonicalize(q.Skills)

	matches := make([]Match, 0, len(atoms))
	for _, atom := range atoms {
		// An atom may point at a role that is gone, or at none at all; both simply score
		// on the atom's own signals.
		var employment *Employment
		if atom.EmploymentID != nil {
			employment = byID[*atom.EmploymentID]
		}
		if s := score(atom, employment, q); s > 0 {
			matches = append(matches, Match{Atom: atom, Employment: employment, Score: s})
		}
	}

	// Ties break on the claim so the order is stable across calls — an agent re-running a
	// search mid-conversation should not see the evidence shuffle.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Atom.Claim < matches[j].Atom.Claim
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

// score rates one atom against a requirement. It is pure — no store, no clock, no model —
// so the ranking rules are testable on their own, which is the only way weights like these
// stay honest.
func score(a Atom, e *Employment, q Query) float64 {
	var total float64

	skills := newSet(a.Skills)
	for _, want := range q.Skills {
		if skills[want] {
			total += skillWeight
			continue
		}
		// The bullet is silent about the technology, but the role ran on it. Real evidence,
		// weaker: "cut message-posting latency" belongs to a MongoDB requirement when the
		// role was MongoDB-based, just below a bullet that says MongoDB.
		if e != nil && newSet(e.Stack)[want] {
			total += stackWeight
		}
	}

	if q.Text != "" {
		wanted := meaningfulTokens(q.Text)
		if len(wanted) > 0 {
			have := newSet(meaningfulTokens(a.Claim + " " + a.Context))
			for _, token := range wanted {
				if have[token] {
					total += wordWeight
				}
			}
		}
	}

	if total == 0 {
		return 0
	}
	if e != nil && e.Current {
		total += currentRole
	}
	if !a.Provenance.Publishable() {
		total *= inferredFactor
	}
	return total
}

// meaningfulTokens splits text into deduplicated, lowercased word tokens with the
// stopwords removed. It reuses ClaimKey's normalization so the requirement and the
// evidence are compared on identical terms.
func meaningfulTokens(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range strings.Fields(ClaimKey(text)) {
		if stopwords[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func newSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}
