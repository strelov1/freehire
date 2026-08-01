package worker_test

import (
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exemptFromBootstrap lists the cmd/ binaries that open a database pool without going
// through worker.Bootstrap, with the reason. Every other pool-opening binary must use
// the shared bootstrap: it is what initializes Sentry, derives the SIGTERM-cancellable
// context, and returns the cleanup that closes the pool. Two backfills drifted out of
// that rule without anything failing, which is why the rule is a test and not a comment.
var exemptFromBootstrap = map[string]string{
	"server": "long-lived daemon, not a run-once worker: owns its own startup and shutdown",
}

// TestPoolOpeningCommandsUseSharedBootstrap derives its population from behaviour rather
// than from a hand-maintained list of worker names, so a new worker is enrolled by
// existing rather than by someone remembering to add it here.
func TestPoolOpeningCommandsUseSharedBootstrap(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "cmd"))
	if err != nil {
		t.Fatalf("read cmd/: %v", err)
	}

	// The population is every binary that reaches the database at all: the compliant ones
	// name worker.Bootstrap and never mention the pool constructor themselves (Bootstrap
	// opens it for them), so looking only for database.Connect/pgxpool would find the
	// violators and nothing else — and the suite would look healthy the moment it was empty.
	var population int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		src, err := packageSource(filepath.Join("..", "..", "cmd", name))
		if err != nil {
			t.Fatalf("read cmd/%s: %v", name, err)
		}
		usesBootstrap := strings.Contains(src, "worker.Bootstrap")
		opensPoolDirectly := strings.Contains(src, "database.Connect") || strings.Contains(src, "pgxpool")
		if !usesBootstrap && !opensPoolDirectly {
			continue // touches no database — a code generator or a file-writing harvest tool
		}
		population++

		if reason, ok := exemptFromBootstrap[name]; ok {
			if usesBootstrap {
				t.Errorf("cmd/%s is listed as exempt (%s) but now uses worker.Bootstrap — drop the exemption", name, reason)
			}
			continue
		}
		if !usesBootstrap {
			t.Errorf("cmd/%s opens a database pool without worker.Bootstrap: it gets no Sentry init, "+
				"no SIGTERM-cancellable context, and its deferred cleanup is at the mercy of os.Exit. "+
				"Convert it to worker.Main(run) + worker.Bootstrap, or add it to exemptFromBootstrap with a reason.", name)
			continue
		}

		// Bootstrap alone is not enough: it registers the pool close and the worker's own
		// telemetry flush as deferred calls, and os.Exit — which log.Fatal calls — runs no
		// deferred function. A worker that bootstraps and then log.Fatals still drops both,
		// on exactly the failed run whose traces would explain it.
		if strings.Contains(src, "log.Fatal") || strings.Contains(src, "os.Exit") {
			t.Errorf("cmd/%s uses worker.Bootstrap but still calls log.Fatal or os.Exit: neither runs a "+
				"deferred function, so the pool close and any buffered telemetry flush are skipped. "+
				"Return a non-zero code from run() and let worker.Main exit.", name)
		}
	}

	// Guards against the population silently emptying — a rename of the bootstrap helper or
	// the pool constructor would otherwise turn this into a test that passes by finding nothing.
	if population < 25 {
		t.Errorf("only %d database-touching commands found; the detection is probably broken", population)
	}
}

// packageSource concatenates the non-test Go sources of one package directory with the
// comments stripped, by re-printing each parsed file. Matching raw text would let a
// `// TODO: move this to worker.Bootstrap` satisfy the check — which is precisely the
// comment someone deferring the migration would leave behind.
func packageSource(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return "", err
		}
		// printer.Fprint on the *ast.File alone omits comments — they live in File.Comments
		// and are only emitted for a printer.CommentedNode.
		if err := printer.Fprint(&b, fset, file); err != nil {
			return "", err
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
