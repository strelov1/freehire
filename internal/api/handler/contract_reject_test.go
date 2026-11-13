package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

// A body the schema refuses is the client's fault, so it is no fault report; but it
// must still be counted, or a broken web client fails every user unseen.
func TestRenderContractRequestError_CountsABodyReject(t *testing.T) {
	before := contractRejectCount(t, "createAPIKey", "body")
	err := &ogenerrors.DecodeRequestError{
		OperationContext: ogenerrors.OperationContext{Name: "CreateAPIKey", ID: "createAPIKey"},
		Err:              errors.New(`decode application/json: "scope": unexpected null`),
	}
	w := httptest.NewRecorder()
	renderContractRequestError(context.Background(), w, nil, err)

	if w.Code != http.StatusBadRequest || w.Body.String() != `{"error":"invalid body"}`+"\n" {
		t.Fatalf("got %d %q, want 400 invalid body", w.Code, w.Body.String())
	}
	if got := contractRejectCount(t, "createAPIKey", "body") - before; got != 1 {
		t.Fatalf("reject counter grew by %v, want 1", got)
	}
}
