package database

import (
	"testing"
	"time"
)

const testDSN = "postgres://u:p@localhost:5432/db"

// TestWithStatementTimeoutReachesTheConnection pins the wire form, not just the call. The
// option is worth nothing unless Postgres actually receives it, and RuntimeParams is the only
// place pgx will carry an arbitrary GUC through to the session.
func TestWithStatementTimeoutReachesTheConnection(t *testing.T) {
	config, err := poolConfig(testDSN)
	if err != nil {
		t.Fatalf("poolConfig: %v", err)
	}
	WithStatementTimeout(30 * time.Second)(config)

	got, ok := config.ConnConfig.RuntimeParams["statement_timeout"]
	if !ok {
		t.Fatal("statement_timeout is absent from RuntimeParams, so the session would carry no timeout at all")
	}
	// Postgres reads a unitless statement_timeout as milliseconds. "30" would be 30ms — a
	// value that looks set and cuts every real query, which is worse than none.
	if got != "30000" {
		t.Errorf("statement_timeout = %q, want %q (milliseconds, as Postgres reads a unitless value)", got, "30000")
	}
}

// TestPoolWithoutTheOptionCarriesNoStatementTimeout is the half that protects the cron
// workers. They share this package and some legitimately run for hours; a default here would
// turn backfill-derive's whole-catalogue walk into a nightly failure, and the failure would
// read as a database fault rather than as a setting.
func TestPoolWithoutTheOptionCarriesNoStatementTimeout(t *testing.T) {
	config, err := poolConfig(testDSN)
	if err != nil {
		t.Fatalf("poolConfig: %v", err)
	}

	if got, ok := config.ConnConfig.RuntimeParams["statement_timeout"]; ok {
		t.Errorf("a pool opened without WithStatementTimeout carries statement_timeout = %q; "+
			"batch workers must keep Postgres's own default", got)
	}
}

// TestWithStatementTimeoutDoesNotDisturbTheConnectionCap guards the seam between the two
// settings this file now applies: the option mutates the SAME config poolConfig has already
// set MaxConns on, so an option that replaced RuntimeParams or the ConnConfig wholesale would
// silently drop the cap that keeps the fleet inside Postgres's connection slots.
func TestWithStatementTimeoutDoesNotDisturbTheConnectionCap(t *testing.T) {
	config, err := poolConfig(testDSN)
	if err != nil {
		t.Fatalf("poolConfig: %v", err)
	}
	WithStatementTimeout(30 * time.Second)(config)

	if config.MaxConns != defaultMaxConns {
		t.Errorf("MaxConns = %d after applying the timeout option, want %d", config.MaxConns, defaultMaxConns)
	}
}
