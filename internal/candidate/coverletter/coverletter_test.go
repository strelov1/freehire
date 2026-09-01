package coverletter

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSanitizeClipsBodyToTheBandCeiling(t *testing.T) {
	b := DefaultBounds()
	l := Letter{Body: strings.Repeat("a", b.StandardCeiling+500)}

	l.Sanitize(BandStandard, b, nil)

	if got := len([]rune(l.Body)); got != b.StandardCeiling {
		t.Errorf("body length = %d, want the standard ceiling %d", got, b.StandardCeiling)
	}
}

func TestSanitizeClipsToTheShortCeilingOnTheShortBand(t *testing.T) {
	b := DefaultBounds()
	l := Letter{Body: strings.Repeat("a", b.StandardCeiling)}

	l.Sanitize(BandShort, b, nil)

	if got := len([]rune(l.Body)); got != b.ShortCeiling {
		t.Errorf("body length = %d, want the short ceiling %d", got, b.ShortCeiling)
	}
}

// The model is handed a set of atoms and asked which it used. It must not be able to
// widen that set: an id it invents would render in the UI as evidence the letter does
// not have, which is the one claim this feature makes.
func TestSanitizeDropsCitationsOutsideTheOfferedSet(t *testing.T) {
	offered := uuid.New()
	invented := uuid.New()
	l := Letter{Body: "ok", Cited: []uuid.UUID{offered, invented}}

	l.Sanitize(BandStandard, DefaultBounds(), []uuid.UUID{offered})

	if len(l.Cited) != 1 || l.Cited[0] != offered {
		t.Errorf("Cited = %v, want only the offered id %v", l.Cited, offered)
	}
}

func TestSanitizeCapsTheCitationCount(t *testing.T) {
	b := DefaultBounds()
	offered := make([]uuid.UUID, b.MaxCited+3)
	for i := range offered {
		offered[i] = uuid.New()
	}
	l := Letter{Body: "ok", Cited: append([]uuid.UUID(nil), offered...)}

	l.Sanitize(BandStandard, b, offered)

	if len(l.Cited) != b.MaxCited {
		t.Errorf("len(Cited) = %d, want the cap %d", len(l.Cited), b.MaxCited)
	}
}

func TestSanitizeDropsDuplicateCitations(t *testing.T) {
	id := uuid.New()
	l := Letter{Body: "ok", Cited: []uuid.UUID{id, id, id}}

	l.Sanitize(BandStandard, DefaultBounds(), []uuid.UUID{id})

	if len(l.Cited) != 1 {
		t.Errorf("len(Cited) = %d, want 1 — a repeated id is one piece of evidence", len(l.Cited))
	}
}

// A nil slice reaching a NOT NULL uuid[] column is sent by pgx as SQL NULL, which a
// column DEFAULT does not cover. Same coercion experience.Sanitize makes, same reason.
func TestSanitizeCoercesNilCitationsToEmpty(t *testing.T) {
	l := Letter{Body: "ok"}

	l.Sanitize(BandStandard, DefaultBounds(), nil)

	if l.Cited == nil {
		t.Error("Cited is nil, want an empty slice — a nil reaches a NOT NULL column as SQL NULL")
	}
}

func TestBelowFloorReportsAnEmptiedLetter(t *testing.T) {
	b := DefaultBounds()

	if !(Letter{Body: strings.Repeat("a", b.Floor-1)}).BelowFloor(b) {
		t.Error("a body under the floor should report below-floor")
	}
	if (Letter{Body: strings.Repeat("a", b.Floor)}).BelowFloor(b) {
		t.Error("a body exactly at the floor should not report below-floor")
	}
}

func TestDefaultBoundsAreInternallyConsistent(t *testing.T) {
	b := DefaultBounds()

	if b.Floor >= b.ShortCeiling {
		t.Errorf("floor %d must sit below the short ceiling %d, or the short band can never pass", b.Floor, b.ShortCeiling)
	}
	if b.ShortCeiling >= b.StandardCeiling {
		t.Errorf("short ceiling %d must sit below the standard ceiling %d", b.ShortCeiling, b.StandardCeiling)
	}
}
