package atsapply

import (
	"context"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// jevSelectClient is the narrow seam JevSelectDrafter needs from Jev; tests fake it.
type jevSelectClient interface {
	Choose(ctx context.Context, question MergedField, options []string, facts []experience.Atom) (answer string, ok bool, err error)
}

// JevSelectDrafter answers select-kind draftable fields through Jev when configured,
// falling back to the wrapped Drafter on any Jev failure. text/textarea fields go to the
// wrapped Drafter unchanged.
type JevSelectDrafter struct {
	LLM Drafter
	// Jev is nil when TYPESAFE_API_KEY is unset; Draft then behaves like the wrapped Drafter.
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

// Draft answers select-kind fields through Jev when configured; any Jev failure falls back
// to the wrapped Drafter instead of surfacing as an error.
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
