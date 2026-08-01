package inbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// recordingLedger notes the order the two statements ran in, which is the whole invariant.
type recordingLedger struct {
	calls       []string
	retractErr  error
	recordErr   error
	gotUserID   int64
	gotEmailID  int64
	gotSource   string
	retractSeen db.RetractSupersededEmailEventParams
}

func (r *recordingLedger) RetractSupersededEmailEvent(_ context.Context, arg db.RetractSupersededEmailEventParams) (int64, error) {
	r.calls = append(r.calls, "retract")
	r.retractSeen = arg
	return 0, r.retractErr
}

func (r *recordingLedger) RecordEmailApplicationEvent(_ context.Context, arg db.RecordEmailApplicationEventParams) error {
	r.calls = append(r.calls, "record")
	r.gotUserID, r.gotEmailID, r.gotSource = arg.UserID, arg.ID, arg.EventSource
	return r.recordErr
}

// The ordering IS the rule. Both statements are data-modifying CTEs reading the same
// pre-statement snapshot, so an insert that runs first still sees the superseded row as live —
// leaving the message with two live events or none. The rule was documented in three places and
// implemented in two, and neither copy was covered by a test: the worker's lived in a cmd/ main,
// outside every domain package's test surface.
func TestReconcileMailEventRetractsBeforeItRecords(t *testing.T) {
	var q recordingLedger

	if err := ReconcileMailEvent(context.Background(), &q, 7, 42, "gmail"); err != nil {
		t.Fatalf("ReconcileMailEvent: %v", err)
	}

	if got := strings.Join(q.calls, ","); got != "retract,record" {
		t.Errorf("statement order = %q, want \"retract,record\" — an insert that runs first sees "+
			"the superseded row as live", got)
	}
	if q.retractSeen.ID != 42 || q.retractSeen.UserID != 7 {
		t.Errorf("retract addressed (id=%d,user=%d), want (42,7)", q.retractSeen.ID, q.retractSeen.UserID)
	}
	if q.gotEmailID != 42 || q.gotUserID != 7 || q.gotSource == "" {
		t.Errorf("record addressed (id=%d,user=%d,source=%q), want (42,7,non-empty)",
			q.gotEmailID, q.gotUserID, q.gotSource)
	}
}

// A failed retraction must stop the reconcile. Recording anyway would insert alongside a
// superseded row nobody retracted — the two-live-events state the ordering exists to prevent.
func TestReconcileMailEventStopsWhenTheRetractionFails(t *testing.T) {
	q := recordingLedger{retractErr: errors.New("deadlock detected")}

	err := ReconcileMailEvent(context.Background(), &q, 7, 42, "gmail")
	if err == nil {
		t.Fatal("a failed retraction was not reported")
	}
	if got := strings.Join(q.calls, ","); got != "retract" {
		t.Errorf("calls = %q, want only \"retract\" — recording after a failed retraction leaves "+
			"two live events", got)
	}
}

// An unknown mail source is rejected before either statement runs, so a bad source cannot
// half-apply the reconcile.
func TestReconcileMailEventRejectsAnUnknownSourceBeforeWriting(t *testing.T) {
	var q recordingLedger

	if err := ReconcileMailEvent(context.Background(), &q, 7, 42, "carrier-pigeon"); err == nil {
		t.Fatal("an unknown mail source was accepted")
	}
	if len(q.calls) != 0 {
		t.Errorf("calls = %v, want none before the source is validated", q.calls)
	}
}
