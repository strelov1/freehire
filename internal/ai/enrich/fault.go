package enrich

import (
	"errors"

	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// errInvalidPayload marks an extraction that failed Validate after Sanitize, i.e. a
// structural failure a fresh sample will not fix.
var errInvalidPayload = errors.New("enrich: invalid payload")

// postingAtFault reports whether the posting itself caused err. A posting's fault spends
// the attempt budget; anything else waits out the grace window (RecordEnrichmentFailure).
//
// It enumerates only failures this package raises, so an unanticipated error class is
// retried rather than dead-lettering a posting that was never at fault.
func postingAtFault(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, errUnparseableResponse):
		// The model could not produce JSON for this input; a retry generally reproduces it.
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
