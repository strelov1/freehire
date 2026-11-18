package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordContractReject(t *testing.T) {
	before := testutil.ToFloat64(contractRejects.WithLabelValues("createAPIKey", "body"))
	RecordContractReject("createAPIKey", "body")
	if got := testutil.ToFloat64(contractRejects.WithLabelValues("createAPIKey", "body")) - before; got != 1 {
		t.Fatalf("increment = %v, want 1", got)
	}
}
