package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ratelimit"
	"github.com/strelov1/freehire/internal/searchintent"
)

// searchIntentsPerHour bounds how many interpretations one caller may run per hour.
//
// Each one is a model call on their own gateway credential, and the refine round makes
// a second. The number is set well above deliberate use — describing a search, refining
// it twice and starting over is a handful of calls, and someone genuinely exploring
// might do that a dozen times in a sitting — while still bounding a script pointed at
// the endpoint. It is not a credit budget: this spends tokens, not AI credits.
const searchIntentsPerHour = 40

// searchIntentLimiter throttles the interpretation endpoint per authenticated caller.
// Keyed on the user, not the address, for the reason matchAnalysisLimiter gives: the
// caller is already authenticated, and an IP key is lifted by any rotating proxy pool.
// Mounted after the auth middleware so the id is resolved.
func searchIntentLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByUserOrIP("searchintent"), searchIntentsPerHour, time.Hour)
}

// intentHandlers serves the search-interpretation endpoint. It is transport only: it
// reads the caller, builds the request, binds the model client to that caller's own
// gateway credential, and renders what internal/searchintent returns. Every decision
// about what a description means lives there, where it is testable without a server.
//
// It reads NO profile, deliberately. Turning a saved profile into filters is already a
// feature — "Apply my profile", filtersFromProfile in web/src/lib/facetModel.ts — and it
// is a pure client-side mapping because a profile is already written in the filter's own
// vocabulary. A second implementation here would be a diverging copy of rules that one
// place already gets right (the base-location gate, the include-wins overlap rule), and
// would spend a model call to do worse.
//
// The model client arrives ONLY through llm: the interpreter is built per request from
// the caller's own bound client. Holding a second, service-credential interpreter
// beside it would be two sources of one dependency, free to disagree about whether the
// feature is configured at all.
type intentHandlers struct {
	llm llmBinding
}

func newIntentHandlers(llm llmBinding) *intentHandlers {
	return &intentHandlers{llm: llm}
}

func (h *intentHandlers) register(api fiber.Router, mw middleware) {
	// Cookie only. This is a browser surface, and keeping it there means the model
	// spend is attached to an interactive session, where a per-user limiter means
	// something. An integration that wants filters has the documented facet vocabulary
	// and /agent/jobs/search; widening this later is additive.
	api.Post("/search/interpret", mw.cookie, searchIntentLimiter(mw.throttler), h.InterpretSearch)
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

// InterpretSearch turns a written description of a job search into filter values.
// Cookie-authenticated: this is a browser surface, and an integration that wants
// filters has the documented facet vocabulary and /agent/jobs/search.
//
// Response: {"data": {...}}. A result that resolved nothing is a 200 carrying
// empty:true, not an error — the request was fine, the description was not usable.
func (h *intentHandlers) InterpretSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
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

	interpreter := searchintent.NewInterpreter(h.llm.bind(c.Context(), userID, tagSearchIntent))
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
