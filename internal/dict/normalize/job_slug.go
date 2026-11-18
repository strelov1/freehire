package normalize

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// shortcodeLen is the number of base32 characters kept from the hash. 8 chars =
// 40 bits. The jobs.public_slug UNIQUE constraint detects collisions, and ingest
// retries those with JobSlugWithFullHash.
const shortcodeLen = 8

// JobSlug builds a job's public slug: the normalized title and company joined
// with a short code derived from the dedup key (source, external_id), e.g.
// "senior-go-developer-acme-t35nijto". Empty title/company segments are dropped
// so the slug never contains an empty segment, and the short code is always
// present so the slug is never empty.
//
// The short code is a deterministic function of (source, external_id) ONLY, so
// re-ingesting the same job (an upsert of the same row) yields the same slug —
// it must not depend on volatile fields like the description.
func JobSlug(title, company, source, externalID string) string {
	segments := make([]string, 0, 3)
	if t := Slug(title); t != "" {
		segments = append(segments, t)
	}
	if c := Slug(company); c != "" {
		segments = append(segments, c)
	}
	segments = append(segments, shortcode(source, externalID))
	return strings.Join(segments, "-")
}

// shortcode hashes source and external_id into a lowercased base32 prefix. The
// NUL separator keeps ("ab","c") and ("a","bc") distinct.
func shortcode(source, externalID string) string {
	return jobSlugHash(source, externalID)[:shortcodeLen]
}

func jobSlugHash(source, externalID string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + externalID))
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
}

// JobSlugWithFullHash resolves a short-hash collision without changing the final
// eight-character identity code used by old-slug redirects. Existing published
// slugs stay untouched; this is only the fallback for a rejected insert.
func JobSlugWithFullHash(title, company, source, externalID string) string {
	slug := JobSlug(title, company, source, externalID)
	hash := jobSlugHash(source, externalID)
	suffix := hash[shortcodeLen:] + "-" + hash[:shortcodeLen]
	prefix := strings.TrimSuffix(slug, hash[:shortcodeLen])
	// Mobile return intents accept at most 160 characters. Only new fallback URLs
	// are bounded; the complete identity suffix and all published URLs stay intact.
	if len(prefix)+len(suffix) > 160 {
		prefix = strings.TrimRight(prefix[:160-len(suffix)-1], "-") + "-"
	}
	return prefix + suffix
}

// JobSlugCode returns the shortcode a job slug ends with — the part JobSlug derives
// from (source, external_id) alone, so it survives any change to the title or company
// the rest of the slug was built from. ok is false when slug does not end with a
// well-formed code, so a caller never looks up a fragment of some other string.
func JobSlugCode(slug string) (code string, ok bool) {
	if len(slug) < shortcodeLen {
		return "", false
	}
	head, code := slug[:len(slug)-shortcodeLen], slug[len(slug)-shortcodeLen:]
	if head != "" && !strings.HasSuffix(head, "-") {
		return "", false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '2' || r > '7') {
			return "", false
		}
	}
	return code, true
}

// JobSlugIdentityCode returns the identity suffix used by a stale link: a full
// hash for collision recovery URLs, or the ordinary eight-character shortcode.
func JobSlugIdentityCode(slug string) (string, bool) {
	code, ok := JobSlugCode(slug)
	if !ok {
		return "", false
	}
	const fullSuffixLen = 53 // 44 remaining base32 hash characters, a hyphen, and 8 shortcode characters.
	if len(slug) < fullSuffixLen {
		return code, true
	}
	suffix := slug[len(slug)-fullSuffixLen:]
	if len(slug) > fullSuffixLen && slug[len(slug)-fullSuffixLen-1] != '-' {
		return code, true
	}
	if suffix[44] != '-' {
		return code, true
	}
	for _, r := range suffix[:44] {
		if (r < 'a' || r > 'z') && (r < '2' || r > '7') {
			return code, true
		}
	}
	return suffix, true
}
