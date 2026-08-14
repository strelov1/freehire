package observability

import "testing"

// StartMetricsServer's listener must bound every phase of a connection, or a
// client that opens one and stalls (slow-loris-style) can hold it open
// indefinitely with no code-level backstop — see metricsServerTimeout's doc.
func TestMetricsServerHasTimeouts(t *testing.T) {
	srv := metricsServer("9999")
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout is unset; want a positive bound")
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout is unset; want a positive bound")
	}
	if srv.WriteTimeout <= 0 {
		t.Error("WriteTimeout is unset; want a positive bound")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout is unset; want a positive bound")
	}
}

// Without a DSN the integration must stay dormant: Init reports no error and
// returns a callable no-op flush, so an unconfigured deployment runs unchanged.
func TestInitDisabledWithoutDSN(t *testing.T) {
	flush, err := Init("", "test")
	if err != nil {
		t.Fatalf("Init(\"\") returned error: %v", err)
	}
	if flush == nil {
		t.Fatal("Init(\"\") returned nil flush; want a no-op flush")
	}
	flush() // must not panic
}

// A malformed DSN is a misconfiguration the caller should fail fast on, so Init
// surfaces the parse error and returns no flush.
func TestInitRejectsMalformedDSN(t *testing.T) {
	flush, err := Init("not-a-valid-dsn", "test")
	if err == nil {
		t.Fatal("Init with a malformed DSN returned nil error; want an error")
	}
	if flush != nil {
		t.Fatal("Init error path should return a nil flush")
	}
}
