package worker

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSignalContextCancelsOnSignal(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	defer stop()

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	select {
	case <-ctx.Done():
		// cancelled as expected
	case <-time.After(2 * time.Second):
		t.Fatal("context not cancelled after SIGTERM")
	}
}

func TestBootstrapFailsFastWhenDatabaseUnreachable(t *testing.T) {
	// Port 1 on loopback refuses immediately, so Connect's ping fails fast
	// without waiting on a timeout and without needing a live database.
	t.Setenv("DATABASE_URL", "postgres://hire:hire@127.0.0.1:1/hire?sslmode=disable")

	ctx, _, pool, cleanup, err := Bootstrap(context.Background())
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected an error when the database is unreachable, got nil")
	}
	if pool != nil {
		t.Fatal("expected a nil pool on bootstrap failure")
	}
	if ctx != nil {
		t.Fatal("expected a nil context on bootstrap failure")
	}
}
