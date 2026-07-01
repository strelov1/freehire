package savedsearch

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/normalize"
)

const (
	// slugSuffixLen is the length of the random suffix appended to a board slug — enough
	// to disambiguate boards sharing a name and to make slugs non-trivially guessable,
	// while keeping the URL short.
	slugSuffixLen = 4
	// slugBaseMaxLen caps the readable, name-derived part of a slug so a long name can't
	// produce an unwieldy URL.
	slugBaseMaxLen = 60
	// slugFallbackBase is used when a name transliterates to nothing (e.g. all symbols),
	// so a board always gets a usable slug.
	slugFallbackBase = "board"
	// maxShareAttempts bounds the retry loop when a minted slug collides with an existing
	// board; each attempt draws a fresh suffix. Collisions are astronomically unlikely, so
	// this is a safety backstop, not a hot path.
	maxShareAttempts = 5
)

// slugAlphabet is the character set for the random suffix: lowercase letters and digits,
// so the suffix stays URL-safe and visually consistent with the transliterated base.
const slugAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// Share publishes a saved search as a public board, owner-scoped. It reads the set (a
// missing or non-owned id → ErrNotFound), keeps an existing public slug on re-share or
// mints a readable one from the name otherwise, and stores the optional author label
// (trimmed; blank → anonymous, over-long → ErrInvalidAuthorLabel). A slug collision is
// retried with a fresh suffix. Returns the updated row (with its public slug).
func (s *Service) Share(ctx context.Context, userID, id int64, authorLabel string) (db.SavedSearch, error) {
	label, err := validAuthorLabel(authorLabel)
	if err != nil {
		return db.SavedSearch{}, err
	}

	set, err := s.repo.Get(ctx, db.GetSavedSearchParams{ID: id, UserID: userID})
	if err != nil {
		return db.SavedSearch{}, err
	}

	// Re-share keeps the existing slug so a previously shared link stays valid.
	if set.PublicSlug.Valid {
		return s.repo.SetPublicSlug(ctx, db.SetSavedSearchPublicSlugParams{
			ID:          id,
			UserID:      userID,
			PublicSlug:  set.PublicSlug,
			AuthorLabel: label,
		})
	}

	for attempt := 0; attempt < maxShareAttempts; attempt++ {
		slug, err := boardSlug(set.Name)
		if err != nil {
			return db.SavedSearch{}, err
		}
		row, err := s.repo.SetPublicSlug(ctx, db.SetSavedSearchPublicSlugParams{
			ID:          id,
			UserID:      userID,
			PublicSlug:  pgtype.Text{String: slug, Valid: true},
			AuthorLabel: label,
		})
		if errors.Is(err, ErrSlugTaken) {
			continue // fresh suffix on the next attempt
		}
		if err != nil {
			return db.SavedSearch{}, err
		}
		return row, nil
	}
	return db.SavedSearch{}, ErrSlugTaken
}

// Unshare makes a shared board private again, owner-scoped. It is an idempotent no-op
// when the set is already private; a missing or non-owned id → ErrNotFound.
func (s *Service) Unshare(ctx context.Context, userID, id int64) error {
	return s.repo.ClearPublicSlug(ctx, db.ClearSavedSearchPublicSlugParams{ID: id, UserID: userID})
}

// GetPublicBoard reads a shared board by its public slug — no auth, no owner-scoping.
// An unknown or unshared slug → ErrNotFound.
func (s *Service) GetPublicBoard(ctx context.Context, slug string) (db.GetPublicBoardBySlugRow, error) {
	return s.repo.GetPublicBoard(ctx, slug)
}

// validAuthorLabel trims the label and enforces the length bound (counted in runes, as
// labels are often Cyrillic). A blank label is valid and stored as NULL (the board renders
// anonymously); an over-long one → ErrInvalidAuthorLabel.
func validAuthorLabel(label string) (pgtype.Text, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return pgtype.Text{}, nil
	}
	if utf8.RuneCountInString(label) > maxAuthorLabelLen {
		return pgtype.Text{}, ErrInvalidAuthorLabel
	}
	return pgtype.Text{String: label, Valid: true}, nil
}

// boardSlug builds a readable public slug from a name: the transliterated, hyphenated base
// (bounded and never empty) plus a random suffix, e.g. "Remote Go" → "remote-go-a3f1".
func boardSlug(name string) (string, error) {
	base := normalize.Slug(name)
	if base == "" {
		base = slugFallbackBase
	}
	if len(base) > slugBaseMaxLen {
		base = strings.TrimRight(base[:slugBaseMaxLen], "-")
	}
	suffix, err := randomSuffix(slugSuffixLen)
	if err != nil {
		return "", err
	}
	return base + "-" + suffix, nil
}

// randomSuffix returns n random characters from slugAlphabet, drawn from a CSPRNG so slugs
// are not enumerable by sequence.
func randomSuffix(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = slugAlphabet[int(b)%len(slugAlphabet)]
	}
	return string(buf), nil
}
