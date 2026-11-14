package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/search/search"
)

// The contract server is the part of the API that TypeSpec describes
// (typespec/, compiled to openapi/api.openapi.json, generated into
// internal/api/apigen). A route there is still a Fiber route: it keeps its own
// middleware chain (auth, limits, the paid gate) and only its last handler is the
// generated server, mounted through the net/http adaptor. Routes move into the
// contract one group at a time; what stays hand-written keeps its Fiber handler.

// contractHandlers is the one value the generated server calls. Each migrated group
// embeds its handlers here, so a group that misses an operation does not compile.
type contractHandlers struct {
	*emailPrefsHandlers
	*talentCatalogHandlers
	*companyWatchHandlers
	*geoHandlers
	*suggestHandlers
	*agentPermissionHandlers
	*usageHandlers
	*plansHandlers
	*dictionaryHandlers
	*jdResolveHandlers
	*screeningAnswersHandlers
	*surveyHandlers
	*voteHandlers
	*ghostReportHandlers
	*talentNetworkHandlers
	*companyFeedbackHandlers
	*companyRelationHandlers
	*reportHandlers
	*answerBankHandlers
	*userModerationHandlers
	*companyProcessReportHandlers
	*contributionHandlers
	*marketPulseHandlers
	*promoHandlers
	*mentorSuggestionsHandlers
	*mentorshipHandlers
	*savedSearchHandlers
	*subscriptionHandlers
	*timelineHandlers
	*profileHandlers
	*conversationHandlers
	*communityHandlers
	*buddyHandlers
	*statsHandlers
	*submissionHandlers
	*trackingHandlers
	*experienceHandlers
	*employerHandlers
	*referralHandlers
	*billingHandlers
	*planHandlers
	*directApplyHandlers
	*jobsHandlers
	*searchHandlers
	*matchHandlers
	*autofillHandlers
	*intentHandlers
	*embedProgressHandlers
	*assistantHandlers
	*companiesHandlers
	*resumeHandlers
	*cvHandlers
	*inboxHandlers
	*authHandlers
}

var _ apigen.Handler = contractHandlers{}

// localsContractFault carries a handler's error out of the generated server, which
// renders it itself, back to the Fiber route, which reports it with the route's
// attributes exactly as RenderError does for a hand-written route.
const localsContractFault = "contract.fault"

type contractFault struct{ err error }

// localsContractQuery carries the raw query into the handlers. The generated server
// decodes only the params the contract names, and an endpoint that widens on a param it
// cannot read has to name the rest in meta.ignored_params.
const localsContractQuery = "contract.query"

// newContract builds the generated server and returns the Fiber handler that ends
// every contract route.
func newContract(h contractHandlers) fiber.Handler {
	srv, err := apigen.NewServer(h,
		apigen.WithPathPrefix("/api/v1"),
		apigen.WithErrorHandler(renderContractRequestError))
	if err != nil {
		// Only the static options above can fail it, so this is a programming error.
		panic("contract server: " + err.Error())
	}
	serve := adaptor.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if target, ok := r.Context().Value(localsContractTarget).(*url.URL); ok {
			r.URL.Path, r.URL.RawPath, r.URL.RawQuery = target.Path, target.RawPath, target.RawQuery
		}
		if user, ok := r.Context().Value(localsContractUserContext).(context.Context); ok {
			r = r.WithContext(withUserContext{Context: r.Context(), user: user})
		}
		srv.ServeHTTP(w, r)
	}))
	return func(c *fiber.Ctx) error {
		fault := &contractFault{}
		c.Locals(localsContractFault, fault)
		// The adaptor parses the request line before anything here runs, and answers one it
		// cannot parse with an unreported 500. A path with a broken escape names nothing.
		if _, err := url.ParseRequestURI(string(c.Request().RequestURI())); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "not found")
		}
		query := queryValues(c)
		// Postgres refuses a NUL byte in any text value, so one that reaches a query is a 500.
		// No route has a use for it; refuse it here, as the decoder refuses a malformed param.
		if hasNUL(c.Request().URI().Path()) || hasNULIn(query) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid params")
		}
		c.Locals(localsContractQuery, query)
		c.Locals(localsContractTarget, fiberTarget(c, query))
		c.Locals(localsContractUserContext, c.UserContext())
		// Fiber routes HEAD to every Get, but the generated router knows only the
		// methods the contract names. Serve it as GET and put HEAD back, so fasthttp,
		// which decides after the handler, still drops the body.
		if c.Method() == fiber.MethodHead {
			c.Request().Header.SetMethod(fiber.MethodGet)
			defer c.Request().Header.SetMethod(fiber.MethodHead)
		}
		if err := serve(c); err != nil {
			return err
		}
		if fault.err != nil {
			reportUnexpected(c, fault.err)
		}
		return nil
	}
}

// NewError renders a handler's error with the same status and message RenderError
// gives a hand-written route, so moving a route into the contract changes no answer.
func (contractHandlers) NewError(ctx context.Context, err error) *apigen.ProblemStatusCode {
	var ce *codedError
	if errors.As(err, &ce) {
		return &apigen.ProblemStatusCode{StatusCode: ce.status,
			Response: apigen.Problem{Error: ce.message, Code: apigen.NewOptString(ce.code)}}
	}
	status, msg, report := classify(err)
	if report {
		recordContractFault(ctx, err)
	}
	return &apigen.ProblemStatusCode{StatusCode: status, Response: apigen.Problem{Error: msg}}
}

// renderContractRequestError answers what the generated server refuses before a
// handler runs (a body or parameter that does not decode) and a response it could
// not encode, in the `{"error": msg}` shape every other refusal has.
func renderContractRequestError(ctx context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	status := ogenerrors.ErrorCode(err)
	msg := "internal server error"
	var badBody *ogenerrors.DecodeRequestError
	var badParams *ogenerrors.DecodeParamsError
	switch {
	case errors.As(err, &badBody):
		msg = "invalid body"
		recordContractReject(badBody.OperationID(), "body", err)
	case errors.As(err, &badParams):
		msg = "invalid params"
		recordContractReject(badParams.OperationID(), "params", err)
	case status < http.StatusInternalServerError:
		msg = http.StatusText(status)
	default:
		recordContractFault(ctx, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(&apigen.Problem{Error: msg}) // the client is gone if this fails
}

// recordContractReject makes a schema refusal visible: a 400 is the client's fault, but
// when the client is our own web app it is our bug, and nothing else would show it.
func recordContractReject(operation, kind string, err error) {
	observability.RecordContractReject(operation, kind)
	reason := err.Error()
	if len(reason) > 300 {
		reason = reason[:300] // shortcut: byte cut may split a rune, fine for a log line
	}
	log.Printf("contract reject: operation=%s kind=%s reason=%q", operation, kind, reason)
}

func recordContractFault(ctx context.Context, err error) {
	if fault, ok := ctx.Value(localsContractFault).(*contractFault); ok {
		fault.err = err
	}
}

// localsContractUserContext carries Fiber's user context to the generated server. Middleware
// marks it (plan.WithPaidAI / WithFreeAI on the paid gate, the trace span), and the request
// context the adaptor builds holds only Fiber Locals, so without it a paid route's model
// call is refused for want of the mark and its faults lose their trace.
const localsContractUserContext = "contract.user_context"

// withUserContext is the request context (cancellation, deadline, Fiber Locals) that also
// answers the values the user context holds.
type withUserContext struct {
	context.Context
	user context.Context
}

func (c withUserContext) Value(key any) any {
	if v := c.user.Value(key); v != nil {
		return v
	}
	return c.Context.Value(key)
}

// localsContractTarget is the URL the generated router is given instead of the one sent.
const localsContractTarget = "contract.target"

// fiberTarget is the request as Fiber already accepted it, for the generated router,
// which is stricter: Fiber matches a route whatever the letter case and with one trailing
// slash, and reads the first of repeated params, while the generated router 404s the
// first two and refuses a repeated scalar param with a 400. The path is the matched
// route's own, with the request's param values put in.
//
// shortcut: every repeated param is cut to its first value, which would also cut an
// exploded array param; declare one only after this keeps the arrays the contract names.
func fiberTarget(c *fiber.Ctx, query url.Values) *url.URL {
	// Path holds the param values decoded and RawPath escaped, which the generated router
	// reads, so an encoded slash in a value stays inside that param.
	segments := strings.Split(c.Route().Path, "/")
	escaped := slices.Clone(segments)
	for i, seg := range segments {
		if name, ok := strings.CutPrefix(seg, ":"); ok {
			value, err := url.PathUnescape(c.Params(name))
			if err != nil {
				value = c.Params(name)
			}
			segments[i], escaped[i] = value, url.PathEscape(value)
		}
	}
	target := &url.URL{
		Path:     strings.Join(segments, "/"),
		RawPath:  strings.Join(escaped, "/"),
		RawQuery: string(c.Request().URI().QueryString()),
	}
	for _, vals := range query {
		if len(vals) > 1 {
			first := make(url.Values, len(query))
			for k, v := range query {
				first[k] = v[:1]
			}
			target.RawQuery = first.Encode()
			break
		}
	}
	return target
}

// contractQuery is the request's whole query, for an operation that reports what it did
// not read.
func contractQuery(ctx context.Context) url.Values {
	vals, _ := ctx.Value(localsContractQuery).(url.Values)
	return vals
}

// contractIgnoredParams is the wire form of search's report: nil when empty, so the
// optional field is left out rather than sent as `[]` (see listResponseWithIgnored).
func contractIgnoredParams(ignored []search.UnknownParam) []apigen.IgnoredParam {
	if len(ignored) == 0 {
		return nil
	}
	out := make([]apigen.IgnoredParam, len(ignored))
	for i, p := range ignored {
		out[i] = apigen.IgnoredParam{Param: p.Param, DidYouMean: optString(p.DidYouMean)}
	}
	return out
}

// optString is an omitempty string: unset when empty.
func optString(s string) apigen.OptString {
	if s == "" {
		return apigen.OptString{}
	}
	return apigen.NewOptString(s)
}

// nilIfEmpty is an omitempty slice: the generated encoder leaves out only a nil one.
func nilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// ptrOf is a nullable optional field as the pointer a domain patch reads: nil when the
// field was absent or null. Pass the field's Get method.
func ptrOf[T any](get func() (T, bool)) *T {
	if v, ok := get(); ok {
		return &v
	}
	return nil
}

// intPtrOf is ptrOf for an integer the domain holds as int.
func intPtrOf(get func() (int64, bool)) *int {
	if v, ok := get(); ok {
		n := int(v)
		return &n
	}
	return nil
}

// optNil is the generated nullable optional field set from a domain pointer: left unset
// for nil, so it is omitted from the answer as `omitempty` did. Pass the field's SetTo.
func optNil[T any](p *T, setTo func(T)) {
	if p != nil {
		setTo(*p)
	}
}

// optNilInt is optNil for a domain int carried as int64.
func optNilInt(p *int, setTo func(int64)) {
	if p != nil {
		setTo(int64(*p))
	}
}

// nilString is an always-present nullable string set from a domain pointer: null for nil.
func nilString(p *string) apigen.NilString {
	if p == nil {
		return apigen.NilString{Null: true}
	}
	return apigen.NewNilString(*p)
}

// nilDateTime is an always-present nullable timestamp set from a domain pointer: null for nil.
func nilDateTime(t *time.Time) apigen.NilDateTime {
	if t == nil {
		return apigen.NilDateTime{Null: true}
	}
	return apigen.NewNilDateTime(*t)
}

// contractIgnoredValues is search's value-level report as the raw object encoding/json wrote
// (keys sorted; a generated map would encode them in random order). Nil when empty, so the
// optional field is left out.
func contractIgnoredValues(values map[string][]string) jx.Raw {
	if len(values) == 0 {
		return nil
	}
	raw, _ := json.Marshal(values) // a map of string slices always marshals
	return raw
}

func hasNUL(b []byte) bool { return bytes.IndexByte(b, 0) >= 0 }

func hasNULIn(query url.Values) bool {
	for k, vals := range query {
		if strings.IndexByte(k, 0) >= 0 {
			return true
		}
		for _, v := range vals {
			if strings.IndexByte(v, 0) >= 0 {
				return true
			}
		}
	}
	return false
}

// int64s widens rollup integers to the contract's int64, never nil: the field is a
// required array, so an absent value is [] rather than null.
func int64s(v []int32) []int64 {
	out := make([]int64, len(v))
	for i, x := range v {
		out[i] = int64(x)
	}
	return out
}
