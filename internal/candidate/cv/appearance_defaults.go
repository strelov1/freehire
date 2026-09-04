package cv

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// AppearanceDefaults is the template/typography/margins a user has saved to seed every new
// base CV with. It carries no document content — only the three things a fresh CV's
// creation call sites otherwise hardcode.
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
// standard CV defaults with ok=false when the user has never saved any — never a zero or
// absent shape, so a caller always has something concrete to seed a new CV from or to show
// on the settings screen. It is the one place every base-CV creation call site (Store.Tailor,
// the reset-from-résumé handler, CreateCV) reads this from, so they cannot drift from one
// another.
//
// A saved template_id that no longer resolves — the template registry (template.go) can
// shrink after it was saved — heals to DefaultTemplateID rather than being handed on as-is:
// Store.Create never validates its templateID argument, so an unresolvable id would otherwise
// reach a new CV silently and only fail much later, at render time. The saved typography and
// margins are unaffected; only the template is stale.
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
// previously saved values, and returns exactly what was saved (already sanitized) so a caller
// never needs a second read to report back what it just wrote. It rejects an unknown
// template_id (ErrUnknownTemplate) and clamps out-of-range typography/margin values — the same
// rules a CV document's own Sanitize applies — rather than rejecting them.
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
