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

func TestErrorCodeOutsideTheStandardsEnumIsRejected(t *testing.T) {
	// Proves the check above can actually fail. An agent branches on this field, so a code of
	// our own invention is unreadable rather than merely unfamiliar.
	if err := validateAgainstSchema(t, schemaErrorResponse, NewError("not_found", "a code we made up")); err == nil {
		t.Fatal("an invented error code validated; the enum is not being enforced")
	}
}
