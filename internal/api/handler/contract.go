package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/ogen-go/ogen/ogenerrors"

	"github.com/strelov1/freehire/internal/api/apigen"
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
}

var _ apigen.Handler = contractHandlers{}

// localsContractFault carries a handler's error out of the generated server, which
// renders it itself, back to the Fiber route, which reports it with the route's
// attributes exactly as RenderError does for a hand-written route.
const localsContractFault = "contract.fault"

type contractFault struct{ err error }

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
	serve := adaptor.HTTPHandler(srv)
	return func(c *fiber.Ctx) error {
		fault := &contractFault{}
		c.Locals(localsContractFault, fault)
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
	case errors.As(err, &badParams):
		msg = "invalid params"
	case status < http.StatusInternalServerError:
		msg = http.StatusText(status)
	default:
		recordContractFault(ctx, err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(&apigen.Problem{Error: msg}) // the client is gone if this fails
}

func recordContractFault(ctx context.Context, err error) {
	if fault, ok := ctx.Value(localsContractFault).(*contractFault); ok {
		fault.err = err
	}
}
