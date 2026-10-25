package candidateprofile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// ProfileReader supplies the job-search profile whose "where I am" base backstops a contact
// block that states no location.
type ProfileReader interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

// WithProfileLocation adds the job-search profile as the last source of the candidate's
// location. A CV or résumé that states one still wins: the base is a search preference the
// candidate set for matching, so it fills the gap rather than overriding what they wrote on
// their CV.
//
// Measured on production 2026-10-25: an approved Workable application parked on its required
// `address` field for a candidate whose CV carries no location at all, while their profile
// said Florianópolis, Brazil.
func (a *Assembler) WithProfileLocation(r ProfileReader) *Assembler {
	a.profiles = r
	return a
}

// baseLocation renders the profile's base as "City, Country", or whichever of the two is
// stated; empty when there is no profile or no base.
func (a *Assembler) baseLocation(ctx context.Context, userID int64) (string, error) {
	if a.profiles == nil {
		return "", nil
	}
	profile, err := a.profiles.Get(ctx, userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(profile.LocationPreferences) == 0 {
		return "", nil
	}
	var prefs userprofile.LocationPreferences
	if err := json.Unmarshal(profile.LocationPreferences, &prefs); err != nil {
		return "", err
	}
	return formatBase(prefs.Base), nil
}

// formatBase joins the base's city and English country name. A country code the language
// tables do not know is left out rather than printed raw: "Florianópolis, br" reads as a typo
// on an application form.
func formatBase(b userprofile.BaseLocation) string {
	parts := make([]string, 0, 2)
	if city := strings.TrimSpace(b.City); city != "" {
		parts = append(parts, city)
	}
	if region, err := language.ParseRegion(strings.TrimSpace(b.Country)); err == nil {
		if name := display.English.Regions().Name(region); name != "" {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, ", ")
}
