package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetryStandbyConflict(t *testing.T) {
	standbyConflictPause = 0
	conflict := &pgconn.PgError{Code: "40001", Message: "canceling statement due to conflict with recovery"}
	other := errors.New("syntax error")

	for _, tc := range []struct {
		name      string
		errs      []error
		wantCalls int
		wantErr   error
	}{
		{"passes first time", []error{nil}, 1, nil},
		{"one conflict is rerun", []error{conflict, nil}, 2, nil},
		{"repeated conflicts fail", []error{conflict, conflict, conflict, nil}, 3, conflict},
		{"other errors are not rerun", []error{other, nil}, 1, other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := RetryStandbyConflict(context.Background(), "q", func() (int, error) {
				err := tc.errs[calls]
				calls++
				return calls, err
			})
			if calls != tc.wantCalls || !errors.Is(err, tc.wantErr) {
				t.Fatalf("calls=%d err=%v, want calls=%d err=%v", calls, err, tc.wantCalls, tc.wantErr)
			}
			if err == nil && got != calls {
				t.Fatalf("result %d, want the last run's %d", got, calls)
			}
		})
	}
}
