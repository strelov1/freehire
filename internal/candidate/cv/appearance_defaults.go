package cv

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// AppearanceDefaults is the template/typography/margins a user has saved to seed every new
// base CV with. It carries no document content.
type AppearanceDefaults struct {
	TemplateID string
	Style      Style
	Margins    Margins
}

func marshalStyle(s Style) ([]byte, error) { return json.Marshal(s) }

func unmarshalStyle(data []byte) (Style, error) {
	var s Style
	if err := json.Unmarshal(data, &s); err != nil {
		return Style{}, err
	}
	return s, nil
}

func marshalMargins(m Margins) ([]byte, error) { return json.Marshal(m) }

func unmarshalMargins(data []byte) (Margins, error) {
	var m Margins
	if err := json.Unmarshal(data, &m); err != nil {
		return Margins{}, err
	}
	return m, nil
}

// GetAppearanceDefaults returns the user's saved appearance defaults, or the system's
// standard defaults with ok=false when none were saved, never a zero shape. It is the one
// place every base-CV creation site reads from, so they cannot drift.
//
// A saved template_id that no longer resolves heals to DefaultTemplateID: Store.Create never
// validates it, so a stale id would fail much later, at render time. Typography and margins
// are unaffected.
func (s *Store) GetAppearanceDefaults(ctx context.Context, userID int64) (AppearanceDefaults, bool, error) {
	row, err := s.repo.GetAppearanceDefaults(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AppearanceDefaults{TemplateID: DefaultTemplateID, Style: Style{}, Margins: DefaultMargins()}, false, nil
		}
		return AppearanceDefaults{}, false, err
	}
	style, err := unmarshalStyle(row.Style)
	if err != nil {
		return AppearanceDefaults{}, false, err
	}
	margins, err := unmarshalMargins(row.Margins)
	if err != nil {
		return AppearanceDefaults{}, false, err
	}
	templateID := row.TemplateID
	if _, err := ResolveTemplate(templateID); err != nil {
		templateID = DefaultTemplateID
	}
	return AppearanceDefaults{TemplateID: templateID, Style: style, Margins: margins}, true, nil
}

// SetAppearanceDefaults validates and stores the user's appearance defaults, replacing any
// saved values, and returns exactly what was saved (sanitized). It rejects an unknown
// template_id (ErrUnknownTemplate) and clamps out-of-range typography/margins like a CV
// document's own Sanitize.
func (s *Store) SetAppearanceDefaults(ctx context.Context, userID int64, defaults AppearanceDefaults) (AppearanceDefaults, error) {
	tmpl, err := ResolveTemplate(defaults.TemplateID)
	if err != nil {
		return AppearanceDefaults{}, err
	}
	saved := AppearanceDefaults{TemplateID: tmpl.ID, Style: defaults.Style.sanitized(), Margins: defaults.Margins.sanitized()}
	style, err := marshalStyle(saved.Style)
	if err != nil {
		return AppearanceDefaults{}, err
	}
	margins, err := marshalMargins(saved.Margins)
	if err != nil {
		return AppearanceDefaults{}, err
	}
	if _, err := s.repo.UpsertAppearanceDefaults(ctx, userID, saved.TemplateID, style, margins); err != nil {
		return AppearanceDefaults{}, err
	}
	return saved, nil
}
