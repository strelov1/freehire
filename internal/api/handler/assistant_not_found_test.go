package handler

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// A slug that names nothing is counted as not_found, with the text the agent already read; a
// real lookup failure is not dressed up as "no vacancy" (get_job and get_company used to turn
// every error into that sentence, and the MCP metrics counted the sentence as internal_error).
func TestLookupMissIsNotFoundAndAFaultStaysAFault(t *testing.T) {
	miss := lookupMiss(pgx.ErrNoRows, `no vacancy with slug "x"`)
	if code := observability.ErrorCode(miss); code != "not_found" || miss.Error() != `no vacancy with slug "x"` {
		t.Errorf("miss: code %q, text %q", code, miss.Error())
	}
	fault := errors.New("connection reset")
	if got := lookupMiss(fault, "no vacancy"); !errors.Is(got, fault) {
		t.Errorf("a database fault became %q", got)
	}
	if code := observability.ErrorCode(trackingToolError("x", jobtracking.ErrJobNotFound)); code != "not_found" {
		t.Errorf("tracking miss: code %q", code)
	}
}
