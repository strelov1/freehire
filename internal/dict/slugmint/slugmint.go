// Package slugmint mints readable, unpredictable public slugs — a transliterated,
// hyphenated base (via normalize.Slug) plus a random suffix. Relocated from
// internal/search/savedsearch's board-sharing code (which minted board slugs) so
// internal/application/joblists can mint job-list slugs from the same logic; it had
// exactly one caller before and has exactly one caller after, so this is a move, not
// new shared infrastructure built ahead of need.
//
// Lives in internal/dict, not internal/platform: it depends on
// internal/dict/normalize for transliteration, and platform sits below dict in the
// layer order, so platform must not import dict. dict sits below both application
// and search (this package's two callers, past and present), so it is reachable from
// both.
package slugmint

import (
	"crypto/rand"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

const (
	// SuffixLen is the length of the random suffix appended to a slug — enough to
	// disambiguate entries sharing a name and to make slugs non-trivially guessable,
	// while keeping the URL short.
	SuffixLen = 4
	// BaseMaxLen caps the readable, name-derived part of a slug so a long name can't
	// produce an unwieldy URL.
	BaseMaxLen = 60
)

// alphabet is the character set for the random suffix: lowercase letters and digits,
// so the suffix stays URL-safe and visually consistent with the transliterated base.
const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// New builds a readable slug from name: the transliterated, hyphenated base (bounded,
// falling back to fallbackBase when name transliterates to nothing) plus a random
// suffix, e.g. New("Remote Go", "list") -> "remote-go-a3f1".
func New(name, fallbackBase string) (string, error) {
	base := normalize.Slug(name)
	if base == "" {
		base = fallbackBase
	}
	if len(base) > BaseMaxLen {
		base = strings.TrimRight(base[:BaseMaxLen], "-")
	}
	suffix, err := randomSuffix(SuffixLen)
	if err != nil {
		return "", err
	}
	return base + "-" + suffix, nil
}

// randomSuffix returns n random characters from alphabet, drawn from a CSPRNG so
// slugs are not enumerable by sequence.
func randomSuffix(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}
