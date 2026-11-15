package searchintent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// ErrDisabled is returned when no model client is configured. The feature is off, which
// must not be shown as "I did not understand you".
var ErrDisabled = errors.New("searchintent: no model configured")

// ErrNothingToInterpret is returned for a request with no description and no previous
// result. It is a bad request, not a server fault, so callers must tell it apart.
var ErrNothingToInterpret = errors.New("searchintent: nothing to interpret")

// MaxTextRunes bounds the description a caller may submit.
const MaxTextRunes = 1000

// Request is one interpretation to run: the caller's text and optionally the result
// being refined.
type Request struct {
	// Text is the caller's own description. There is no Profile field: FromProfile builds
	// a profile's search without a model; adjust it by passing the result as Previous.
	Text string
	// Previous is the result being refined. The model returns a complete replacement, not
	// a diff.
	Previous *Result
}

// Interpreter turns a description into a Result over an llm.Client.
type Interpreter struct {
	client *llm.Client
}

// NewInterpreter wraps a model client. A nil client disables the feature.
func NewInterpreter(client *llm.Client) *Interpreter {
	return &Interpreter{client: client}
}

// Enabled reports whether interpretation can run at all.
func (i *Interpreter) Enabled() bool { return i != nil && i.client != nil }

// Interpret asks the model for one proposal and returns it resolved. The summary comes
// back in the same call as the values, so the two cannot disagree.
func (i *Interpreter) Interpret(ctx context.Context, req Request) (Result, error) {
	if !i.Enabled() {
		return Result{}, ErrDisabled
	}
	user, err := userPrompt(req)
	if err != nil {
		return Result{}, err
	}
	schema, err := requestSchema()
	if err != nil {
		return Result{}, err
	}
	raw, err := i.client.GenerateJSON(ctx, systemPrompt, user, llm.WithSchema(schemaName, schema), llm.WithReasoning(llm.ReasoningNone))
	if err != nil {
		return Result{}, fmt.Errorf("searchintent: generate: %w", err)
	}
	var p proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Result{}, fmt.Errorf("searchintent: parse: %w", err)
	}
	return p.intent().resolve()
}

// userPrompt states what to interpret. An empty request is refused instead of paying
// for a model call that can only invent.
func userPrompt(req Request) (string, error) {
	text := strings.TrimSpace(req.Text)
	if len([]rune(text)) > MaxTextRunes {
		return "", fmt.Errorf("searchintent: description is longer than %d characters", MaxTextRunes)
	}

	if text == "" && req.Previous == nil {
		return "", ErrNothingToInterpret
	}

	var b strings.Builder
	if req.Previous != nil {
		b.WriteString("The search so far:\n")
		b.WriteString(describe(req.Previous.reground()))
		b.WriteString("\nChange it as follows, and return the WHOLE search, not just the change:\n")
	} else {
		b.WriteString("Build a search from this description:\n")
	}
	b.WriteString(text)
	return b.String(), nil
}

// describe renders a previous result as the text the model reads, so a refinement works
// from the live search, not its own earlier prose.
func describe(r Result) string {
	var b strings.Builder
	for _, name := range sortedFacetNames(r.Facets) {
		fmt.Fprintf(&b, "- %s: %s\n", name, strings.Join(r.Facets[name], ", "))
	}
	// Exclusions must be shown too, or the model's replacement silently drops them
	// ("remote, not in the USA" + "also senior" would put the USA back).
	for _, name := range sortedFacetNames(r.Exclude) {
		fmt.Fprintf(&b, "- NOT %s: %s\n", name, strings.Join(r.Exclude[name], ", "))
	}
	if r.Query != "" {
		fmt.Fprintf(&b, "- free text: %s\n", r.Query)
	}
	if r.Scalars.SalaryMin != nil {
		fmt.Fprintf(&b, "- salary at least: %d\n", *r.Scalars.SalaryMin)
	}
	if r.Scalars.PostedWithinDays != nil {
		fmt.Fprintf(&b, "- posted within days: %d\n", *r.Scalars.PostedWithinDays)
	}
	if r.Scalars.ExperienceYearsMax != nil {
		fmt.Fprintf(&b, "- experience at most (years): %d\n", *r.Scalars.ExperienceYearsMax)
	}
	if r.Scalars.VisaSponsorship {
		b.WriteString("- visa sponsorship required\n")
	}
	return b.String()
}
