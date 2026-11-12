package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/searchintent"
)

// intentHandlers serves the search-interpretation endpoint. It is transport only: it
// reads the caller, builds the request, binds the model client to that caller's own
// gateway credential, and renders what internal/search/searchintent returns. Every decision
// about what a description means lives there, where it is testable without a server.
//
// It reads NO profile, deliberately. Turning a saved profile into filters is already a
// feature — "Apply my profile", filtersFromProfile in web/src/lib/facetModel.ts — and it
// is a pure client-side mapping because a profile is already written in the filter's own
// vocabulary. A second implementation here would be a diverging copy of rules that one
// place already gets right (the base-location gate, the include-wins overlap rule), and
// would spend a model call to do worse.
//
// The model client arrives ONLY through llm: the interpreter is built per request — from
// the caller's own bound client when signed in, from that same field's service client
// when not. Holding a second, service-credential interpreter beside it would be two
// sources of one dependency, free to disagree about whether the feature is configured at
// all.
type intentHandlers struct {
	llm llmBinding
}

func newIntentHandlers(llm llmBinding) *intentHandlers {
	return &intentHandlers{llm: llm}
}

func (h *intentHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Optional session: open to signed-out visitors as well. The AI search is how a
	// visitor first finds the catalogue, and asking them to sign in before they have seen
	// it hid the feature from exactly the people it is for. A signed-in caller is unchanged
	// — their own credential, their own per-user budget; a guest is answered on the service
	// credential. An integration that
	// wants filters still has the documented facet vocabulary and /agent/jobs/search.
	//
	// Deliberately NOT behind mw.paidAI: the interpretation is one of the model calls every
	// account gets (plan.WithFreeAI, marked in the handler). It is a short call on a cheap
	// model. Every call answered 402 to free on 2026-10-19.
	api.Post("/search/interpret", mw.optional, refuseEncodedGuestBody, contract)
}

// interpretResult is the wire shape of one interpretation. It round-trips: the dialog
// sends the previous result back verbatim to refine it, so the two directions are one
// type rather than two that can drift.
type interpretResult struct {
	Summary            string              `json:"summary"`
	Facets             map[string][]string `json:"facets"`
	Exclude            map[string][]string `json:"exclude"`
	Query              string              `json:"query"`
	SalaryMin          *int                `json:"salary_min"`
	PostedWithinDays   *int                `json:"posted_within_days"`
	ExperienceYearsMax *int                `json:"experience_years_max"`
	VisaSponsorship    bool                `json:"visa_sponsorship"`
	// Unresolved names what the interpretation could not place, verbatim as the model
	// wrote it. Rendered to the caller: a drop they are not told about is
	// indistinguishable from a value that was applied.
	Unresolved []string `json:"unresolved"`
	// Empty says nothing resolved. It is reported rather than left to the caller to
	// infer from empty fields, because an empty filter shows the whole catalogue and
	// reads as "everything matches you" instead of "I did not understand".
	Empty bool `json:"empty"`
}

func resultView(r searchintent.Result) interpretResult {
	return interpretResult{
		Summary:            r.Summary,
		Facets:             r.Facets,
		Exclude:            r.Exclude,
		Query:              r.Query,
		SalaryMin:          r.Scalars.SalaryMin,
		PostedWithinDays:   r.Scalars.PostedWithinDays,
		ExperienceYearsMax: r.Scalars.ExperienceYearsMax,
		VisaSponsorship:    r.Scalars.VisaSponsorship,
		Unresolved:         r.Unresolved,
		Empty:              r.Empty(),
	}
}

func (v interpretResult) result() searchintent.Result {
	return searchintent.Result{
		Summary: v.Summary,
		Facets:  v.Facets,
		Exclude: v.Exclude,
		Query:   v.Query,
		Scalars: searchintent.Scalars{
			SalaryMin:          v.SalaryMin,
			PostedWithinDays:   v.PostedWithinDays,
			ExperienceYearsMax: v.ExperienceYearsMax,
			VisaSponsorship:    v.VisaSponsorship,
		},
	}
}

// InterpretSearch turns a written description of a job search into filter values. Open
// to signed-in and signed-out callers alike (see register).
//
// Response: {"data": {...}}. A result that resolved nothing is a 200 carrying
// empty:true, not an error — the request was fine, the description was not usable.
func (h *intentHandlers) InterpretSearch(ctx context.Context, body *apigen.SearchIntentRequest) (*apigen.InterpretSearchOK, error) {
	userID, signedIn := auth.UserIDFromContext(ctx)
	// The previous result travels as raw JSON and is decoded here, so a malformed one is
	// the same "invalid request body" a malformed request always was.
	var previous *interpretResult
	if len(body.Previous) > 0 {
		if err := json.Unmarshal(body.Previous, &previous); err != nil {
			return nil, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
	}
	text := body.Text.Value
	if len([]rune(text)) > searchintent.MaxTextRunes {
		return nil, fiber.NewError(fiber.StatusBadRequest, "description is too long")
	}

	req := searchintent.Request{Text: text}
	if previous != nil {
		prev := previous.result()
		req.Previous = &prev
	}

	model, err := h.model(ctx, userID, signedIn)
	if err != nil {
		return nil, err
	}
	interpreter := searchintent.NewInterpreter(model)
	res, err := interpreter.Interpret(ctx, req)
	switch {
	case errors.Is(err, searchintent.ErrDisabled):
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "AI search is not available")
	case errors.Is(err, searchintent.ErrNothingToInterpret):
		return nil, fiber.NewError(fiber.StatusBadRequest, "describe the search you are looking for")
	case err != nil:
		return nil, err
	}
	raw, err := json.Marshal(resultView(res))
	if err != nil {
		return nil, err
	}
	return &apigen.InterpretSearchOK{Data: raw}, nil
}

// refuseEncodedGuestBody refuses a signed-out caller's encoded body before the contract
// decodes it (see refuseEncodedBody): the generated server cannot see the header.
func refuseEncodedGuestBody(c *fiber.Ctx) error {
	if _, signedIn := auth.UserID(c); !signedIn {
		if err := refuseEncodedBody(c); err != nil {
			return err
		}
	}
	return c.Next()
}

// model is the client one interpretation runs on.
//
// A signed-in caller spends as themselves, through the binding. A guest spends the
// service client, tagged with the feature, and NEVER goes through the binding: there is
// no account to attribute to, and llmkey's resolver asked about account 0 would read and
// then MINT a gateway credential for it — a key nobody owns, created on every guest call.
func (h *intentHandlers) model(ctx context.Context, userID int64, signedIn bool) (*llm.Client, error) {
	if signedIn {
		return h.llm.bind(plan.WithFreeAI(ctx), userID, llm.Feature(tagSearchIntent))
	}
	return h.llm.client.As("", nil, llm.Feature(tagSearchIntent)), nil
}
