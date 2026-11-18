package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// accountsError must leave a fault it does not recognise to classify, so a server fault is
// reported and a client cancel is 499 — the same answer the error would get on any route.
func TestAccountsErrorLeavesFaultsToClassify(t *testing.T) {
	for name, err := range map[string]error{
		"own deadline":      fmt.Errorf("get user: %w", context.DeadlineExceeded),
		"client cancel":     fmt.Errorf("get user: %w", context.Canceled),
		"pool exhausted":    errors.New("failed to acquire connection from pool"),
		"rows closed (bug)": errors.New("sql: Rows are closed"),
	} {
		wantStatus, _, wantReport := classify(err)
		gotStatus, _, gotReport := classify(accountsError(err))
		if gotStatus != wantStatus || gotReport != wantReport {
			t.Errorf("%s: got %d report=%v, want %d report=%v", name, gotStatus, gotReport, wantStatus, wantReport)
		}
	}
}
