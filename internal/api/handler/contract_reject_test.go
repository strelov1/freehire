package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/prometheus/client_golang/prometheus"
)

func contractRejectCount(t *testing.T, operation, kind string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "freehire_contract_rejects_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["operation"] == operation && labels["kind"] == kind {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// A body or params the schema refuses is the client's fault, so it is no fault report;
// but it must still be counted, or a broken web client fails every user unseen.
func TestRenderContractRequestError_CountsRejects(t *testing.T) {
	op := ogenerrors.OperationContext{Name: "CreateAPIKey", ID: "createAPIKey"}
	cases := []struct {
		kind, want string
		err        error
	}{
		{"body", "invalid body", &ogenerrors.DecodeRequestError{OperationContext: op, Err: errors.New(strings.Repeat("x", 400))}},
		{"params", "invalid params", &ogenerrors.DecodeParamsError{OperationContext: op, Err: errors.New("bad")}},
	}
	for _, tc := range cases {
		before := contractRejectCount(t, "createAPIKey", tc.kind)
		w := httptest.NewRecorder()
		renderContractRequestError(context.Background(), w, nil, tc.err)

		if w.Code != http.StatusBadRequest || w.Body.String() != `{"error":"`+tc.want+`"}`+"\n" {
			t.Errorf("%s: got %d %q, want 400 %s", tc.kind, w.Code, w.Body.String(), tc.want)
		}
		if got := contractRejectCount(t, "createAPIKey", tc.kind) - before; got != 1 {
			t.Errorf("%s: reject counter grew by %v, want 1", tc.kind, got)
		}
	}
}
