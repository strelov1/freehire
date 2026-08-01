package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// The Telegram post's timestamp is the posting's source posted time: it is supplied
// by the caller rather than derived, and must reach the written params intact.
func TestBuildParams_CarriesSuppliedPostedTime(t *testing.T) {
	posted := pgtype.Timestamptz{Time: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), Valid: true}

	params, err := buildParams(
		"telegram", "chan/42", "https://t.me/chan/42",
		"Senior Go Developer", "Acme", "Berlin", false,
		"We use Golang and PostgreSQL.", "", posted,
	)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	if !params.PostedAt.Valid || !params.PostedAt.Time.Equal(posted.Time) {
		t.Errorf("PostedAt = %v, want %v", params.PostedAt, posted.Time)
	}
}

// An extraction with no post timestamp leaves posted_at unset rather than stamping
// a zero instant, so the freshness signal can tell "unknown" from "the epoch".
func TestBuildParams_UnsetPostedTimeStaysNull(t *testing.T) {
	params, err := buildParams(
		"telegram", "chan/43", "https://t.me/chan/43",
		"Backend Engineer", "Acme", "Berlin", false,
		"Go and Postgres.", "", pgtype.Timestamptz{},
	)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	if params.PostedAt.Valid {
		t.Errorf("PostedAt = %v, want unset", params.PostedAt)
	}
}
