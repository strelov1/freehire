package database

import (
	"context"
	"errors"
	"testing"
	"time"
)

// poolConfig must cap MaxConns when the DSN is silent (so the worker fleet can't
// exhaust Postgres slots) but yield to an explicit pool_max_conns override.
func TestPoolConfig_MaxConns(t *testing.T) {
	t.Run("caps to default when DSN omits pool_max_conns", func(t *testing.T) {
		cfg, err := poolConfig("postgres://u:p@localhost:5432/db")
		if err != nil {
			t.Fatalf("poolConfig: %v", err)
		}
		if cfg.MaxConns != defaultMaxConns {
			t.Errorf("MaxConns = %d, want %d", cfg.MaxConns, defaultMaxConns)
		}
	})

	t.Run("respects an explicit pool_max_conns", func(t *testing.T) {
		cfg, err := poolConfig("postgres://u:p@localhost:5432/db?pool_max_conns=30")
		if err != nil {
			t.Fatalf("poolConfig: %v", err)
		}
		if cfg.MaxConns != 30 {
			t.Errorf("MaxConns = %d, want 30", cfg.MaxConns)
		}
	})

	t.Run("a password containing the text is not an override", func(t *testing.T) {
		cfg, err := poolConfig("postgres://u:pool_max_conns%3D99@localhost:5432/db")
		if err != nil {
			t.Fatalf("poolConfig: %v", err)
		}
		if cfg.MaxConns != defaultMaxConns {
			t.Errorf("MaxConns = %d, want %d (the substring sits in the password, not the settings)", cfg.MaxConns, defaultMaxConns)
		}
	})
}

// A database that never answers must not hold the caller past its own deadline. This is the
// guard on the retry loop's cost: a cron worker with a wrong DSN should fail its slot promptly,
// not sit for the full window — and a worker cancelled by SIGTERM mid-startup stops now.
func TestConnectStopsWhenTheCallerDoes(t *testing.T) {
	// Port 1 has nothing listening, so every ping is refused immediately and the loop is
	// exercised at full speed.
	const dsn = "postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1"

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	start := time.Now()
	pool, err := Connect(ctx, dsn)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("Connect succeeded against a port with no listener")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to carry the caller's deadline", err)
	}
	// Generous, but far below reachableWindow: the point is that the caller's context wins.
	if elapsed > 5*time.Second {
		t.Errorf("Connect took %s; the caller's 400ms deadline did not stop the retry loop", elapsed)
	}
}

// The window is what bounds a database that is merely slow to arrive. Kept as an explicit
// assertion because raising it is a real trade — longer waits for a restarting database, longer
// held cron slots for a misconfigured one — and should be a deliberate edit.
func TestReachableWindowIsBounded(t *testing.T) {
	if reachableWindow < 5*time.Second || reachableWindow > time.Minute {
		t.Errorf("reachableWindow = %s; outside the range this was reasoned about", reachableWindow)
	}
}

// The whole point is that it RETRIES — a single ping is what lost the race against Postgres's
// init scripts. "Connection refused" comes back instantly, so a non-retrying Connect returns in
// milliseconds; a retrying one keeps going until its window or the caller's deadline closes.
// Elapsed time is therefore the honest observation, and it fails against the old implementation.
func TestConnectRetriesRatherThanFailingOnTheFirstPing(t *testing.T) {
	// Port 1 has nothing listening, so each attempt is refused immediately.
	const dsn = "postgres://nobody:nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1"
	const window = 1200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), window)
	defer cancel()

	start := time.Now()
	pool, err := Connect(ctx, dsn)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("Connect succeeded against a port with no listener")
	}
	if elapsed < window/2 {
		t.Errorf("Connect gave up after %s against an instantly-refused port; it is not retrying", elapsed)
	}
}
