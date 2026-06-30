// Package jobhash fingerprints the job fields that form the search document, so
// the ingest write path can tell whether a re-ingest actually changed a job's
// searchable content. The fingerprint is stored in jobs.content_hash; a re-ingest
// whose hash matches the stored one needs no re-push to the live search index.
package jobhash

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// Of returns a deterministic hex fingerprint of the indexed fields in p — every
// value that ends up in the Meilisearch document (see internal/search.FromJob).
// The identity columns (source, external_id) are excluded: they are constant for a
// given row, not searchable content, so they must not move the change signal. A
// change to any indexed field changes the hash.
func Of(p db.UpsertJobParams) string {
	// Record-separated so content cannot shift across field boundaries and collide
	// (e.g. title "ab"+company "c" vs title "a"+company "bc"). Slices use a nested
	// unit separator; their order is the deterministic order jobderive produced.
	const rs = "\x1e"
	var b strings.Builder
	write := func(s string) { b.WriteString(s); b.WriteString(rs) }

	write(p.URL)
	write(p.Title)
	write(p.Company)
	write(p.CompanySlug)
	write(p.Location)
	write(strconv.FormatBool(p.Remote))
	write(p.Description)
	write(timestamp(p.PostedAt))
	write(p.PublicSlug)
	write(strings.Join(p.Countries, "\x1f"))
	write(strings.Join(p.Regions, "\x1f"))
	write(p.WorkMode)
	write(strings.Join(p.Skills, "\x1f"))
	write(p.Seniority)
	write(p.Category)
	write(p.PostingLanguage)
	write(p.EmploymentType)
	write(p.EducationLevel)
	write(nullableInt(p.ExperienceYearsMin))

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// timestamp renders an optional timestamp deterministically; an unset value is the
// empty string, distinct from any real instant.
func timestamp(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return strconv.FormatInt(t.Time.UnixNano(), 10)
}

// nullableInt renders an optional int; an unset value is the empty string, distinct
// from any number (including 0).
func nullableInt(n pgtype.Int4) string {
	if !n.Valid {
		return ""
	}
	return strconv.FormatInt(int64(n.Int32), 10)
}
