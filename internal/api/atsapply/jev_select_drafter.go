package atsapply

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// jevSelectClient is the narrow seam JevSelectDrafter needs from a Jev backend — small
// enough that tests substitute a fake without touching the network.
type jevSelectClient interface {
	Choose(ctx context.Context, question MergedField, options []string, facts []experience.Atom) (answer string, ok bool, err error)
}

// JevSelectDrafter answers a select-kind draftable field through Jev when configured,
// falling back to the wrapped Drafter on any Jev failure. text/textarea fields are
// delegated to the wrapped Drafter unchanged — see design.md for why Jev does not fit
// free-text generation.
type JevSelectDrafter struct {
	LLM Drafter
	// Jev is nil when TYPESAFE_API_KEY is unset, so Draft never attempts a Jev call for
	// any field and behaves exactly like the wrapped Drafter alone.
	Jev jevSelectClient
}

// NewJevSelectDrafter builds a JevSelectDrafter, wiring up Jev only when apiKey is
// non-empty.
func NewJevSelectDrafter(llm Drafter, apiKey string) JevSelectDrafter {
	d := JevSelectDrafter{LLM: llm}
	if apiKey != "" {
		d.Jev = newJevSelectChooser(apiKey)
	}
	return d
}

// Draft answers select-kind fields through Jev when configured; any Jev-side failure —
// network, auth, a response shape it cannot read — falls back to the wrapped Drafter
// rather than surfacing as an error, so a third-party outage degrades to today's behavior
// instead of leaving the field unmapped that the LLM could have answered.
func (d JevSelectDrafter) Draft(ctx context.Context, question MergedField, grounding GroundingContext) (string, bool, error) {
	if question.Kind == "select" && d.Jev != nil {
		options := make([]string, 0, len(question.Options))
		for _, opt := range question.Options {
			options = append(options, opt.Label)
		}
		if answer, ok, err := d.Jev.Choose(ctx, question, options, grounding.Atoms); err == nil {
			return answer, ok, nil
		}
	}
	return d.LLM.Draft(ctx, question, grounding)
}
