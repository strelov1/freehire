package talentnetwork

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// mintAttempts bounds the re-mint loop when a freshly minted handle collides with one
// another account already holds. The suffix draws from ~1M values per base, so two
// collisions in a row is already implausible; the bound is here so a store that fails
// every claim ends the request instead of the loop.
const mintAttempts = 5

// JoinStore is what Join needs from *db.Queries.
type JoinStore interface {
	GetTalentNetworkVisibility(ctx context.Context, id int64) (db.GetTalentNetworkVisibilityRow, error)
	GetUserResumeStructuredOnly(ctx context.Context, id int64) ([]byte, error)
	SetTalentHandleIfUnset(ctx context.Context, arg db.SetTalentHandleIfUnsetParams) (int64, error)
}

// Join makes sure the given account has a catalogue handle, minting one if it does not.
//
// It lives here rather than in the handler because what to name somebody, and what to do
// when that name is taken, is a fact about the catalogue — not about HTTP. The handler
// calls it and writes the visibility; this decides the address.
//
// Idempotent by the claim's own `talent_handle IS NULL` predicate, which is what makes
// leaving and rejoining keep the URL a candidate already shared: the second join claims
// nothing and the stored handle stands. A collision with ANOTHER account's handle comes
// back as a unique violation and is answered by minting a new suffix — the same
// allocate-and-retry shape internal/identity/accounts uses for usernames.
func Join(ctx context.Context, store JoinStore, userID int64) error {
	row, err := store.GetTalentNetworkVisibility(ctx, userID)
	if err != nil {
		return err
	}
	if row.TalentHandle.Valid && row.TalentHandle.String != "" {
		return nil
	}

	title, err := primaryTitleOf(ctx, store, userID)
	if err != nil {
		return err
	}

	for range mintAttempts {
		handle, err := MintHandle(title)
		if err != nil {
			return err
		}
		_, err = store.SetTalentHandleIfUnset(ctx, db.SetTalentHandleIfUnsetParams{
			ID:           userID,
			TalentHandle: pgtype.Text{String: handle, Valid: true},
		})
		switch {
		case err == nil:
			// Zero rows means the account already had a handle — a concurrent join won
			// the race — which is the same success as having written one.
			return nil
		case pgerr.IsUniqueViolation(err):
			continue
		default:
			return err
		}
	}
	return fmt.Errorf("talentnetwork: could not mint a free handle in %d attempts", mintAttempts)
}

// primaryTitleOf reads the account's stored CV and returns the title of the role that
// best describes what they do now.
//
// An unreadable or absent CV yields "" and is NOT an error: HandleBase turns it into the
// neutral base, because being unable to name somebody's discipline is not a reason to
// refuse them a URL — and gating membership on the CV pipeline would mean a candidate who
// joins during an extraction cannot.
func primaryTitleOf(ctx context.Context, store JoinStore, userID int64) (string, error) {
	raw, err := store.GetUserResumeStructuredOnly(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	var structured resumeextract.Structured
	if err := json.Unmarshal(raw, &structured); err != nil {
		// A stored structure we cannot parse is a broken extract, not a broken join.
		return "", nil
	}
	return PrimaryTitle(structured), nil
}
