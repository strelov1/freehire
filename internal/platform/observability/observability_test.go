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
