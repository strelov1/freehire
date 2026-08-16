package enrich

import (
	"errors"

	"github.com/strelov1/freehire/internal/pgerr"
)

// errInvalidPayload marks an extraction that failed Validate. Sanitize has already
// dropped out-of-vocabulary values by then, so what remains is structural: this
// posting's extraction is wrong in a way a fresh sample will not fix.
var errInvalidPayload = errors.New("enrich: invalid payload")

// postingAtFault reports whether the posting itself caused err, which decides how the
// failed entry is bounded: a posting's own fault spends the attempt budget, anything
// else waits out the grace window (see RecordEnrichmentFailure).
//
// It enumerates the failures THIS package raises, not the ones upstream returns. Two
// reasons. The upstream's error text belongs to langchaingo and is not ours to depend
// on. And more importantly it puts the default on the right side: an error class nobody
// anticipated is not the posting's fault, so it is retried rather than buried.
//
// That default is the whole point. The previous policy blamed the posting for every
// failure, and two LiteLLM outages in July 2026 permanently dead-lettered 172,875
// postings — every one of them enrichable, none of them at fault. Being wrong towards
// "not the posting's fault" costs some LLM calls; being wrong the other way loses
// postings from the catalogue with no path back.
func postingAtFault(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, errUnparseableResponse):
		// The model could not produce JSON for this input. A retry draws a fresh
		// sample against the same input and generally reproduces it.
		return true
	case errors.Is(err, errInvalidPayload):
		return true
	case pgerr.IsDataCorrupted(err):
		// The row will never load, whatever else is healthy.
		return true
	default:
		return false
	}
}
