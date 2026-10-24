package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/search/searchintent"
)

// searchIntentsPerHour bounds how many interpretations one caller may run per hour.
//
// Each one is a model call on their own gateway credential, and the refine round makes
// a second. The number is set well above deliberate use — describing a search, refining
// it twice and starting over is a handful of calls, and someone genuinely exploring
// might do that a dozen times in a sitting — while still bounding a script pointed at
// the endpoint. It is not a credit budget: this spends tokens, not AI credits.
const searchIntentsPerHour = 40

// guestSearchIntentsPerHour bounds how many interpretations one signed-out address may
// run per hour. Lower than the member budget: a guest spends the SERVICE credential, an
// address is a weaker identity than an account, and ten is still a describe, two
// refinements and a couple of fresh starts. A first guess; revisit against the
// search-intent spend once guests have used it for a week.
const guestSearchIntentsPerHour = 10

// searchIntentLimiter throttles the interpretation endpoint with two separate budgets: a
// signed-in caller per user (an IP key is lifted by any rotating proxy pool, the reason
// matchAnalysisLimiter gives), a guest per address, since there is no one else to key
// by. Mounted after mw.optional so the id is resolved. The member key is the one this
// limiter has always used, so a deploy resets nobody's count.
func searchIntentLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return memberOrGuestLimiter(
		ratelimit.Middleware(throttler, ratelimit.KeyByUserOrIP("searchintent"), searchIntentsPerHour, time.Hour),
		ratelimit.Middleware(throttler, ratelimit.KeyByIP("searchintent-guest"), guestSearchIntentsPerHour, time.Hour),
	)
}

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

func (h *intentHandlers) register(api fiber.Router, mw middleware) {
	// Optional session: open to signed-out visitors as well. The AI search is how a
	// visitor first finds the catalogue, and asking them to sign in before they have seen
	// it hid the feature from exactly the people it is for. A signed-in caller is unchanged
	// — their own credential, their own per-user budget; a guest is answered on the service
	// credential under a per-address budget (searchIntentLimiter). An integration that
	// wants filters still has the documented facet vocabulary and /agent/jobs/search.
	//
	// Deliberately NOT behind mw.paidAI: the interpretation is one of the model calls every
	// account gets (plan.WithFreeAI, marked in the handler). It is a short call on a cheap
	// model, bounded by the limiter. Every call answered 402 to free on 2026-10-19.
	api.Post("/search/interpret", mw.optional, searchIntentLimiter(mw.throttler), h.InterpretSearch)
}

// interpretRequest is what the dialog posts: a description, optionally refining a
// result the caller is already looking at.
type interpretRequest struct {
	Text string `json:"text"`
	// Previous is the result being refined, echoed back from a prior response.
	Previous *interpretResult `json:"previous"`
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
func (h *intentHandlers) InterpretSearch(c *fiber.Ctx) error {
	userID, signedIn := auth.UserID(c)
	if !signedIn {
		if err := refuseEncodedBody(c); err != nil {
			return err
		}
	}
	var in interpretRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len([]rune(in.Text)) > searchintent.MaxTextRunes {
		return fiber.NewError(fiber.StatusBadRequest, "description is too long")
	}

	req := searchintent.Request{Text: in.Text}
	if in.Previous != nil {
		previous := in.Previous.result()
		req.Previous = &previous
	}

	model, err := h.model(c, userID, signedIn)
	if err != nil {
		return err
	}
	interpreter := searchintent.NewInterpreter(model)
	res, err := interpreter.Interpret(c.Context(), req)
	switch {
	case errors.Is(err, searchintent.ErrDisabled):
		return fiber.NewError(fiber.StatusServiceUnavailable, "AI search is not available")
	case errors.Is(err, searchintent.ErrNothingToInterpret):
		return fiber.NewError(fiber.StatusBadRequest, "describe the search you are looking for")
	case err != nil:
		return err
	}
	return c.JSON(fiber.Map{"data": resultView(res)})
}

// model is the client one interpretation runs on.
//
// A signed-in caller spends as themselves, through the binding. A guest spends the
// service client, tagged with the feature, and NEVER goes through the binding: there is
// no account to attribute to, and llmkey's resolver asked about account 0 would read and
// then MINT a gateway credential for it — a key nobody owns, created on every guest call.
func (h *intentHandlers) model(c *fiber.Ctx, userID int64, signedIn bool) (*llm.Client, error) {
	if signedIn {
		return h.llm.bind(plan.WithFreeAI(c.UserContext()), userID, llm.Feature(tagSearchIntent))
	}
	return h.llm.client.As("", nil, llm.Feature(tagSearchIntent)), nil
}
