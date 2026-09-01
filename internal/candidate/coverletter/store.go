package coverletter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Stored is a letter as it sits in the database: the letter itself plus the stamps that decide
// whether it still speaks for the pair it was written for.
type Stored struct {
	Letter
	// Model produced the body. A gateway upgrade retires every letter written by the old one.
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Stale reports whether the stored letter still matches the live world: the configured model,
// and the language the vacancy is now detected as.
//
// The language side resolves the posting through LanguageOf rather than comparing the raw
// column. A posting that carries no detected language resolves to English, and a letter
// correctly written in English against exactly that posting must not report stale forever —
// which is what a raw "" != "en" comparison would do to about 5% of open rows.
func (s Stored) Stale(liveModel, livePostingLanguage string) bool {
	return s.Model != liveModel || s.Language != LanguageOf(livePostingLanguage)
}

// Repository is the narrow port the Store needs. It is owner-scoped by construction: every
// method takes the caller's id, and the SQL behind it filters on that id, so an entry the
// caller does not own reports as missing rather than as forbidden.
type Repository interface {
	Get(ctx context.Context, userID, jobID int64) (Stored, error)
	Upsert(ctx context.Context, userID, jobID int64, s Stored) error
}

// Store is the owner-scoped domain surface over the draft table.
type Store struct{ repo Repository }

func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// Get returns the caller's stored letter for a vacancy, or nil when the pair was never
// drafted. An absent draft is not an error: the read path reports "none" and calls no model,
// which is the whole reason GET and POST are separate verbs here.
func (s *Store) Get(ctx context.Context, userID, jobID int64) (*Stored, error) {
	row, err := s.repo.Get(ctx, userID, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("coverletter: get draft: %w", err)
	}
	return &row, nil
}

// Save replaces the caller's letter for a vacancy, stamping it with the model that wrote it.
// There is one row per pair and no history — see the migration for why a letter does not earn
// cvedit's revisions.
func (s *Store) Save(ctx context.Context, userID, jobID int64, l Letter, model string) error {
	if err := s.repo.Upsert(ctx, userID, jobID, Stored{Letter: l, Model: model}); err != nil {
		return fmt.Errorf("coverletter: save draft: %w", err)
	}
	return nil
}
