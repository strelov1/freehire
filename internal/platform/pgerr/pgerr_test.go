package pgerr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

func TestClassifiersRecogniseTheirOwnSQLSTATEAndNothingElse(t *testing.T) {
	tests := []struct {
		name                            string
		err                             error
		unique, fk, serial, dead, corpt bool
	}{
		{name: "nil"},
		{name: "plain error", err: errors.New("boom")},
		{name: "unique violation", err: &pgconn.PgError{Code: "23505"}, unique: true},
		{name: "foreign key violation", err: &pgconn.PgError{Code: "23503"}, fk: true},
		{name: "serialization failure", err: &pgconn.PgError{Code: "40001"}, serial: true},
		// 40P01 is a sibling of 40001, not the same code: both say "nothing was wrong
		// with the work, run it again", and a classifier that conflated them would let a
		// caller retry one while treating the other as fatal.
		{name: "deadlock detected", err: &pgconn.PgError{Code: "40P01"}, dead: true},
		{name: "wrapped deadlock", err: fmt.Errorf("apply batch: %w", &pgconn.PgError{Code: "40P01"}), dead: true},
		{name: "data corrupted", err: &pgconn.PgError{Code: "XX001"}, corpt: true},
		// Wrapping is the case that matters in practice: a repository returns
		// fmt.Errorf("read batch: %w", err) and the caller still has to classify it.
		{name: "wrapped data corrupted", err: fmt.Errorf("read batch: %w", &pgconn.PgError{Code: "XX001"}), corpt: true},
		{name: "wrapped unique violation", err: fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505"}), unique: true},
		{name: "an unrelated pg error is none of them", err: &pgconn.PgError{Code: "42703"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUniqueViolation(tt.err); got != tt.unique {
				t.Errorf("IsUniqueViolation = %v, want %v", got, tt.unique)
			}
			if got := IsForeignKeyViolation(tt.err); got != tt.fk {
				t.Errorf("IsForeignKeyViolation = %v, want %v", got, tt.fk)
			}
			if got := IsSerializationFailure(tt.err); got != tt.serial {
				t.Errorf("IsSerializationFailure = %v, want %v", got, tt.serial)
			}
			if got := IsDeadlock(tt.err); got != tt.dead {
				t.Errorf("IsDeadlock = %v, want %v", got, tt.dead)
			}
			if got := IsDataCorrupted(tt.err); got != tt.corpt {
				t.Errorf("IsDataCorrupted = %v, want %v", got, tt.corpt)
			}
		})
	}
}

func TestUniqueViolationConstraint(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantName string
		wantOk   bool
	}{
		{name: "nil"},
		{name: "plain error", err: errors.New("boom")},
		{name: "non-unique pg error", err: &pgconn.PgError{Code: "23503", ConstraintName: "fk_x"}},
		{
			name:     "unique violation carries the constraint name",
			err:      &pgconn.PgError{Code: "23505", ConstraintName: "saved_searches_user_id_name_key"},
			wantName: "saved_searches_user_id_name_key",
			wantOk:   true,
		},
		{
			name:     "wrapped unique violation",
			err:      fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: "saved_searches_derived_from_profile_idx"}),
			wantName: "saved_searches_derived_from_profile_idx",
			wantOk:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ok := UniqueViolationConstraint(tt.err)
			if ok != tt.wantOk || name != tt.wantName {
				t.Errorf("UniqueViolationConstraint(%v) = (%q, %v), want (%q, %v)", tt.err, name, ok, tt.wantName, tt.wantOk)
			}
		})
	}
}

// This package's doc calls it "the single home for the SQLSTATE constants". That claim was
// false: internal/platform/worker declared XX001 and repeated the errors.As unwrap, and the predicted
// consequence had already happened — internal/ai/enrich and internal/ai/embed pulled in the whole
// bootstrap dependency tree (config, database, observability) to classify one SQLSTATE.
//
// So the claim is a test now. A package that unwraps *pgconn.PgError is classifying, whatever
// it calls the function; that is this package's job, and a second copy is how the taxonomy
// splits again.
func TestOnlyThisPackageUnwrapsAPgError(t *testing.T) {
	const marker = "pgconn.PgError"

	var scanned, offenders int
	// Anchored at the module root rather than at a counted "../..": the block move made
	// that two levels short of the repo, so the walk silently stopped covering cmd/ while
	// still scanning enough files to clear the vacuous-pass floor below.
	root, rootErr := modroot.Find()
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// A dot- or underscore-prefixed directory is not part of this module: the go
			// toolchain never descends into one, so `./...` cannot compile what lives there.
			// This is a filesystem walk, not a package walk, and without the same rule it
			// reads every git worktree checked out INSIDE the repo (.worktrees,
			// .claude/worktrees) as another copy of this package — one offender per
			// worktree, none of them real, and the count below then fails on a clean tree.
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			// Vendored and generated trees are nobody's taxonomy to keep.
			if name == "node_modules" || name == "archive" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		if !strings.Contains(string(body), marker) {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "internal/platform/pgerr/") {
			return nil
		}
		offenders++
		t.Errorf("%s unwraps *pgconn.PgError; SQLSTATE classification belongs in internal/platform/pgerr, "+
			"whose doc claims to be its single home. Add a predicate there and call it.", path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if scanned < 100 {
		t.Fatalf("only %d Go files scanned — the walk is not seeing the repo, so a second "+
			"classifier would pass unnoticed", scanned)
	}
	_ = offenders
}
