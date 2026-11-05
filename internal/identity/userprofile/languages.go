package userprofile

import (
	"errors"
	"strings"
)

// ErrInvalidJobLanguage rejects malformed posting-language codes.
var ErrInvalidJobLanguage = errors.New("userprofile: invalid job language")

// normalizeJobLanguages accepts ISO 639-1 code shapes, matching the posting-language facet.
// Empty means any language. Preserve first-seen order and bound the persisted set.
func normalizeJobLanguages(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		code := strings.ToLower(strings.TrimSpace(raw))
		if code == "" {
			continue
		}
		if !isCountryCode(code) {
			return nil, ErrInvalidJobLanguage
		}
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	if len(out) > 200 {
		return nil, ErrInvalidJobLanguage
	}
	return out, nil
}
