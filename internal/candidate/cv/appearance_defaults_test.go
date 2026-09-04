package cv

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

func (f *fakeRepo) GetAppearanceDefaults(_ context.Context, userID int64) (db.CvAppearanceDefault, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.appearanceDefaults[userID]
	if !ok {
		return db.CvAppearanceDefault{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeRepo) UpsertAppearanceDefaults(_ context.Context, userID int64, templateID string, style, margins []byte) (db.CvAppearanceDefault, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appearanceDefaults == nil {
		f.appearanceDefaults = map[int64]db.CvAppearanceDefault{}
	}
	row := db.CvAppearanceDefault{UserID: userID, TemplateID: templateID, Style: style, Margins: margins, UpdatedAt: stamp()}
	f.appearanceDefaults[userID] = row
	return row, nil
}

func TestStoreGetAppearanceDefaultsFallsBackToSystemDefaults(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	got, ok, err := s.GetAppearanceDefaults(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false: user has saved nothing")
	}
	want := AppearanceDefaults{TemplateID: DefaultTemplateID, Style: Style{}, Margins: DefaultMargins()}
	if got != want {
		t.Errorf("defaults = %+v, want system defaults %+v", got, want)
	}
}

func TestStoreGetAppearanceDefaultsReturnsSaved(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	style, err := marshalStyle(Style{FontFamily: "inter", FontSize: 11, LineHeight: 0.5})
	if err != nil {
		t.Fatalf("marshal style: %v", err)
	}
	margins, err := marshalMargins(Margins{Top: 0.75, Right: 0.5, Bottom: 0.75, Left: 0.5})
	if err != nil {
		t.Fatalf("marshal margins: %v", err)
	}
	if _, err := repo.UpsertAppearanceDefaults(ctx, 7, "centered", style, margins); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, ok, err := s.GetAppearanceDefaults(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true: user has saved defaults")
	}
	want := AppearanceDefaults{
		TemplateID: "centered",
		Style:      Style{FontFamily: "inter", FontSize: 11, LineHeight: 0.5},
		Margins:    Margins{Top: 0.75, Right: 0.5, Bottom: 0.75, Left: 0.5},
	}
	if got != want {
		t.Errorf("defaults = %+v, want %+v", got, want)
	}
}

// A template can be retired from the registry (template.go's `templates` slice) after a user
// saved it as their default. GetAppearanceDefaults must self-heal to the system default rather
// than hand a since-unresolvable template_id to a creation call site — Store.Create never
// validates its templateID argument, so an unresolvable id would otherwise be written straight
// into a new CV and only fail much later, at render time.
func TestStoreGetAppearanceDefaultsHealsAStaleTemplate(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	style, _ := marshalStyle(Style{FontSize: 11})
	margins, _ := marshalMargins(Margins{Top: 0.75, Right: 0.5, Bottom: 0.75, Left: 0.5})
	if _, err := repo.UpsertAppearanceDefaults(ctx, 7, "retired-template", style, margins); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, ok, err := s.GetAppearanceDefaults(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true: the style/margins are still valid, only the template is stale")
	}
	if got.TemplateID != DefaultTemplateID {
		t.Errorf("templateID = %q, want healed to system default %q", got.TemplateID, DefaultTemplateID)
	}
	if got.Style.FontSize != 11 {
		t.Errorf("style = %+v, want the saved typography preserved", got.Style)
	}
}

func TestStoreGetAppearanceDefaultsIsPerUser(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	style, _ := marshalStyle(Style{FontSize: 12})
	margins, _ := marshalMargins(DefaultMargins())
	if _, err := repo.UpsertAppearanceDefaults(ctx, 1, "modern-sans", style, margins); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, ok, err := s.GetAppearanceDefaults(ctx, 2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false: user 2 saved nothing, user 1 did")
	}
}

func TestStoreSetAppearanceDefaultsRoundTrips(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	in := AppearanceDefaults{
		TemplateID: "sidebar",
		Style:      Style{FontFamily: "liberation-sans", FontSize: 11, LineHeight: 0.5},
		Margins:    Margins{Top: 0.75, Right: 0.5, Bottom: 0.75, Left: 0.5},
	}
	returned, err := s.SetAppearanceDefaults(ctx, 7, in)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if returned != in {
		t.Errorf("SetAppearanceDefaults returned %+v, want the saved value %+v", returned, in)
	}

	got, ok, err := s.GetAppearanceDefaults(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got != in {
		t.Errorf("defaults = %+v, want %+v", got, in)
	}
}

func TestStoreSetAppearanceDefaultsRejectsUnknownTemplate(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	_, err := s.SetAppearanceDefaults(ctx, 7, AppearanceDefaults{TemplateID: "not-a-real-template"})
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("err = %v, want ErrUnknownTemplate", err)
	}

	if _, ok, _ := s.GetAppearanceDefaults(ctx, 7); ok {
		t.Errorf("defaults were saved despite the unknown template")
	}
}

func TestStoreSetAppearanceDefaultsClampsOutOfRangeValues(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	in := AppearanceDefaults{
		TemplateID: "classic-ats",
		Style:      Style{FontSize: 99, LineHeight: 99},
		Margins:    Margins{Top: 99, Right: -5, Bottom: 99, Left: -5},
	}
	if _, err := s.SetAppearanceDefaults(ctx, 7, in); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, ok, err := s.GetAppearanceDefaults(ctx, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got.Style.FontSize != maxFontSize {
		t.Errorf("font size = %v, want clamped to %v", got.Style.FontSize, maxFontSize)
	}
	if got.Style.LineHeight != maxLineHeight {
		t.Errorf("line height = %v, want clamped to %v", got.Style.LineHeight, maxLineHeight)
	}
	if got.Margins.Top != maxMargin || got.Margins.Bottom != maxMargin {
		t.Errorf("margins = %+v, want top/bottom clamped to %v", got.Margins, maxMargin)
	}
	if got.Margins.Right != minMargin || got.Margins.Left != minMargin {
		t.Errorf("margins = %+v, want right/left clamped to %v", got.Margins, minMargin)
	}
}
