package ojcp

import "testing"

// The error envelope is the one shape this package declared a schema constant for and then
// never validated against — which is why it shipped in a form the standard does not
// describe at all. A declared-but-unused schema constant is not dead code, it is a
// switched-off check, and from the inside it looks exactly like a working one.

func TestErrorEnvelopeConformsToTheStandard(t *testing.T) {
	for _, code := range []string{
		ErrorJobNotFound, ErrorEmployerNotFound, ErrorInvalidRequest, ErrorProviderError,
	} {
		t.Run(code, func(t *testing.T) {
			if err := validateAgainstSchema(t, schemaErrorResponse, NewError(code, "something went wrong")); err != nil {
				t.Fatalf("error envelope rejected: %v", err)
			}
		})
	}
}

func TestRateLimitErrorAlwaysSaysHowLongToWait(t *testing.T) {
	// The schema makes retry_after_seconds mandatory alongside `rate_limited` — found by the
	// oracle the moment it was finally pointed at this envelope. An agent told to slow down
	// and not told for how long can only guess, and a guessing agent retries.
	if err := validateAgainstSchema(t, schemaErrorResponse, NewRateLimitError(30)); err != nil {
		t.Fatalf("rate-limit envelope rejected: %v", err)
	}
	if err := validateAgainstSchema(t, schemaErrorResponse, NewError(ErrorRateLimited, "slow down")); err == nil {
		t.Fatal("a rate-limit refusal with no retry_after_seconds validated")
	}
}

func TestErrorCodeOutsideTheStandardsEnumIsRejected(t *testing.T) {
	// Proves the check above can actually fail. An agent branches on this field, so a code of
	// our own invention is unreadable rather than merely unfamiliar.
	if err := validateAgainstSchema(t, schemaErrorResponse, NewError("not_found", "a code we made up")); err == nil {
		t.Fatal("an invented error code validated; the enum is not being enforced")
	}
}
