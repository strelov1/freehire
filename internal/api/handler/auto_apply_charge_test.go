package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// A charge that fails for a reason other than a refusal still blocks the action — an
// unmetered auto-apply submits a real application — but the failure must reach the
// exception log, which classify only does for a 500.
func TestChargeAutoApply_FaultRefusesAndIsReported(t *testing.T) {
	cause := errors.New("failed to acquire connection from pool")
	h := &assistantHandlers{
		plans: plan.NewStore(db.New(failingDBTX{err: cause}), nil, plan.DefaultConfig()),
	}

	err := h.chargeAutoApply(context.Background(), 7, 42)
	if err == nil {
		t.Fatal("a failed charge let the auto-apply through")
	}
	var refused *autoApplyAllowanceRefusal
	if errors.As(err, &refused) {
		t.Fatalf("a fault was rendered as an allowance refusal: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("the cause was dropped: %v", err)
	}
	if _, _, report := classify(err); !report {
		t.Errorf("the fault is not reported: classify(%v) report = false", err)
	}
}
