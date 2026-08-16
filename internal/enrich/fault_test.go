package enrich

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The classifier decides whether a failed entry spends its attempt budget or waits out
// a grace window. Getting it wrong towards "the posting's fault" is what dead-lettered
// 172,875 postings during two LiteLLM outages in July 2026, so the table below carries
// the real production error strings rather than invented ones.
func TestPostingAtFault(t *testing.T) {
	corrupted := &pgconn.PgError{Code: "XX001", Message: "uncorrected data corruption"}

	for _, tc := range []struct {
		name string
		err  error
		want bool
		why  string
	}{
		{
			name: "unparseable model response",
			err:  fmt.Errorf("%w: json: cannot unmarshal bool into Go struct field", errUnparseableResponse),
			want: true,
			why:  "the model could not answer for THIS input; a retry draws the same input",
		},
		{
			name: "invalid payload",
			err:  fmt.Errorf("%w: enrich: invalid seniority %q", errInvalidPayload, "wizard"),
			want: true,
			why:  "the extraction for this posting is structurally wrong, not unlucky",
		},
		{
			name: "corrupted row",
			err:  fmt.Errorf("load job: %w", corrupted),
			want: true,
			why:  "the row will never load",
		},
		{
			name: "gateway 502",
			err:  fmt.Errorf("enrich: llm: generate: %w", errors.New("API returned unexpected status code: 502")),
			want: false,
			why:  "128,744 postings died to exactly this during the July outages",
		},
		{
			name: "gateway 500",
			err:  fmt.Errorf("enrich: llm: generate: %w", errors.New("API returned unexpected status code: 500")),
			want: false,
			why:  "43,528 postings died to exactly this",
		},
		{
			name: "gateway 401",
			err:  fmt.Errorf("enrich: llm: generate: %w", errors.New("API returned unexpected status code: 401: Authentication")),
			want: false,
			why:  "a credential misconfiguration is ours, and it was fixed",
		},
		{
			name: "request timeout",
			err:  fmt.Errorf("enrich: llm: generate: %w", errors.New("request timeout: API call exceeded deadline")),
			want: false,
			why:  "the gateway was slow, not the posting",
		},
		{
			name: "write-back failure",
			err:  fmt.Errorf("write back: %w", errors.New("connection refused")),
			want: false,
			why:  "our database, not the posting",
		},
		{
			name: "unrecognised error",
			err:  errors.New("something nobody has seen before"),
			want: false,
			why:  "the safe default: an unanticipated class must not bury postings",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := postingAtFault(tc.err); got != tc.want {
				t.Errorf("postingAtFault(%v) = %v, want %v — %s", tc.err, got, tc.want, tc.why)
			}
		})
	}
}

// A nil error is not a failure at all. Guarding it here keeps every caller from
// having to.
func TestPostingAtFaultNil(t *testing.T) {
	if postingAtFault(nil) {
		t.Error("postingAtFault(nil) = true; a nil error is not the posting's fault")
	}
}
