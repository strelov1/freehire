package talentnetwork

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// mintAttempts bounds the re-mint loop on handle collisions, so a store that fails every
// claim ends the request instead of looping.
const mintAttempts = 5

// JoinStore is what Join needs from *db.Queries.
type JoinStore interface {
	GetTalentNetworkVisibility(ctx context.Context, id int64) (db.GetTalentNetworkVisibilityRow, error)
	GetUserResumeStructuredOnly(ctx context.Context, id int64) ([]byte, error)
	SetTalentHandleIfUnset(ctx context.Context, arg db.SetTalentHandleIfUnsetParams) (int64, error)
}

// Join makes sure the given account has a catalogue handle, minting one if it does not.
//
// Idempotent via the claim's `talent_handle IS NULL` predicate, so leaving and rejoining
// keeps the URL already shared. A collision with ANOTHER account's handle comes back as a
// unique violation and is re-minted, like internal/identity/accounts usernames.
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
			TalentHandle: pgconv.TextValid(handle),
		})
		switch {
		case err == nil:
			// Zero rows means a concurrent join won the race: still success.
			return nil
		case pgerr.IsUniqueViolation(err):
			continue
		default:
			return err
		}
	}
	return fmt.Errorf("talentnetwork: could not mint a free handle in %d attempts", mintAttempts)
}

// primaryTitleOf returns the title of the role that best describes what the account's
// stored CV says they do now. An unreadable or absent CV yields "" and is NOT an error:
// HandleBase falls back to the neutral base, and membership must not depend on the CV pipeline.
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
