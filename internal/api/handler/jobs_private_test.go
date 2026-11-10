package handler

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestJobVisibleToCaller(t *testing.T) {
	public := db.Job{}
	if err := jobVisibleToCaller(public, 0, false); err != nil {
		t.Fatalf("public anonymous: %v", err)
	}

	owner := pgtype.Int8{Int64: 7, Valid: true}
	private := db.Job{IsPrivate: true, CreatedBy: owner, Description: "secret JD"}

	if err := jobVisibleToCaller(private, 0, false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private anonymous err = %v, want ErrNoRows", err)
	}
	if err := jobVisibleToCaller(private, 8, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private stranger err = %v, want ErrNoRows", err)
	}
	if err := jobVisibleToCaller(private, 7, true); err != nil {
		t.Fatalf("private owner: %v", err)
	}

	orphan := db.Job{IsPrivate: true}
	if err := jobVisibleToCaller(orphan, 7, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("private without created_by err = %v, want ErrNoRows", err)
	}
}
