package jobtracking

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestTrackedJobIDHidesPrivateJobsFromStrangers(t *testing.T) {
	public := db.Job{ID: 1}
	got, err := trackedJobID(public, 9)
	if err != nil || got != 1 {
		t.Fatalf("public: id=%d err=%v", got, err)
	}

	owner := pgtype.Int8{Int64: 7, Valid: true}
	private := db.Job{ID: 2, IsPrivate: true, CreatedBy: owner}

	if _, err := trackedJobID(private, 8); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("stranger err=%v, want ErrJobNotFound", err)
	}
	got, err = trackedJobID(private, 7)
	if err != nil || got != 2 {
		t.Fatalf("owner: id=%d err=%v", got, err)
	}

	orphan := db.Job{ID: 3, IsPrivate: true}
	if _, err := trackedJobID(orphan, 7); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("orphan err=%v, want ErrJobNotFound", err)
	}
}
