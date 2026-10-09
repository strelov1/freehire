package autofillagent

import "context"

// jevClient is the narrow seam JevPlanner needs from a Jev backend — small enough that
// tests substitute a fake without touching the network.
type jevClient interface {
	Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error)
}

// JevPlanner answers Choose through Jev when configured, falling back to the wrapped LLM
// planner on any Jev failure. Plan is untouched: it mixes free-text generation with
// discrete choice in one call, which does not fit Jev's Choice question shape — see
// design.md for why this was not pursued.
type JevPlanner struct {
	LLM Planner
	// Jev is nil when TYPESAFE_API_KEY is unset, so Choose never attempts a Jev call and
	// behaves exactly like the wrapped LLM planner alone.
	Jev jevClient
}

// NewJevPlanner builds a JevPlanner, wiring up Jev only when apiKey is non-empty.
func NewJevPlanner(llm Planner, apiKey string) JevPlanner {
	p := JevPlanner{LLM: llm}
	if apiKey != "" {
		p.Jev = newJevChooser(apiKey)
	}
	return p
}

func (p JevPlanner) Plan(ctx context.Context, fields []Field, profile Profile) ([]Fill, error) {
	return p.LLM.Plan(ctx, fields, profile)
}

// Choose answers through Jev when configured. Any Jev-side failure — network, auth, a
// response shape Choose cannot read — falls back to the LLM rather than surfacing as an
// error, so a third-party outage degrades to today's behavior instead of leaving the
// field unanswered.
func (p JevPlanner) Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error) {
	if p.Jev != nil {
		if choice, err := p.Jev.Choose(ctx, question, options, profile); err == nil {
			return choice, nil
		}
	}
	return p.LLM.Choose(ctx, question, options, profile)
}
