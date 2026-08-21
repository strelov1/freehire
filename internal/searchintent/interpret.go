package searchintent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// ErrDisabled is returned when the deployment has no model client. The feature is off,
// which is a different answer from "I did not understand you" and must not be shown as
// one.
var ErrDisabled = errors.New("searchintent: no model configured")

// ErrNothingToInterpret is returned for a request carrying no description, no profile
// and no previous result. It is a sentinel rather than a plain error because the
// caller's transport has to tell it apart from a fault: this is a bad request, and
// rendering it as a server error would report the caller's empty box as our bug.
var ErrNothingToInterpret = errors.New("searchintent: nothing to interpret")

// MaxTextRunes bounds the description a caller may submit. Two or three sentences
// describe a job search completely; past that the text is either pasted noise or an
// attempt to spend someone else's tokens.
const MaxTextRunes = 1000

// Request is one interpretation to run: what the caller wrote, optionally what we
// already know about them, and optionally the result they are refining.
type Request struct {
	// Text is the caller's own description.
	//
	// There is deliberately no Profile field. A saved profile is already written in the
	// filter's vocabulary, so FromProfile builds that search with no model at all; a
	// caller who wants to adjust one passes the built result back as Previous.
	Text string
	// Previous is the result being refined. Carrying it lets the model return a
	// complete replacement rather than a diff, so the caller always sees one coherent
	// search.
	Previous *Result
}

// Interpreter turns a description into a Result over an llm.Client.
type Interpreter struct {
	client *llm.Client
}

// NewInterpreter wraps a model client. A nil client disables the feature rather than
// failing at call time in a way that reads as a broken request.
func NewInterpreter(client *llm.Client) *Interpreter {
	return &Interpreter{client: client}
}

// As rebinds the interpreter to a client bound to one caller's gateway credential, so
// the spend is theirs. The receiver is not mutated: an interpreter is shared by every
// request, and a per-caller client written into it would leak across them.
func (i *Interpreter) As(client *llm.Client) *Interpreter {
	if i == nil {
		return nil
	}
	return &Interpreter{client: client}
}

// Enabled reports whether interpretation can run at all.
func (i *Interpreter) Enabled() bool { return i != nil && i.client != nil }

// Interpret asks the model for one proposal and returns it resolved. One call: the
// summary the caller is shown comes back with the values, so the two cannot disagree.
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
	raw, err := i.client.GenerateJSON(ctx, systemPrompt, user, llm.WithSchema(schemaName, schema))
	if err != nil {
		return Result{}, fmt.Errorf("searchintent: generate: %w", err)
	}
	var p proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Result{}, fmt.Errorf("searchintent: parse: %w", err)
	}
	return p.intent().resolve()
}

// userPrompt states what to interpret. It refuses a request with nothing in it rather
// than paying for a model call that can only invent — an empty description is a bug in
// the caller, not a search.
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

// describe renders a previous result back as the plain text the model reads, so a
// refinement argues with the search that is actually live rather than with its own
// earlier prose.
func describe(r Result) string {
	var b strings.Builder
	for _, name := range sortedFacetNames(r.Facets) {
		fmt.Fprintf(&b, "- %s: %s\n", name, strings.Join(r.Facets[name], ", "))
	}
	// The exclusions belong here as much as the inclusions do. Left out, the model never
	// learns about them and hands back a replacement that quietly drops them — so
	// refining "remote, not in the USA" with "also senior" puts the USA back, and the
	// person reads a summary saying so one screen after reading the opposite.
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
