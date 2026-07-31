// Package jobdedup answers one question for every write path that persists a posting:
// does the catalogue already hold this same vacancy under a different board identity?
//
// A company routinely posts one role once per city — same title, same description,
// thirty external_ids. RoleFingerprint deliberately excludes the location, so those
// copies share a fingerprint and the batch recompute
// (db.RecomputeRoleDuplicatesForCompany) collapses them to one canonical row. The batch
// runs every few hours, though, and the live search index is written incrementally by
// the crawl; between the two, every copy is a separate searchable job. That window is
// what this package closes, by answering the same question synchronously at write time.
//
// The answer it gives is deliberately the one the batch would give, not merely a
// defensible one — see CanonicalForRole.
package jobdedup

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
)

// CanonicalForRole reports the open canonical posting of this row's role cluster, when
// one exists that is OLDER than the row just written (id).
//
// The age condition is the whole subtlety. RecomputeRoleDuplicatesForCompany makes
// MIN(id) among a cluster's open rows the canon, so collapsing onto a NEWER posting is
// not just a different choice — the next reindex would undo it and invert it. Staying in
// step with the batch is what makes the two agree instead of fighting.
//
// Callers gate which rows are worth asking about; this asks the question. Two things are
// never worth asking about and are refused here, because getting either wrong strands a
// row permanently:
//   - an empty fingerprint, which clusters with nobody;
//   - an empty company slug. The batch recompute drives off a list of company slugs and
//     skips those rows entirely, so a row marked here would never be released when its
//     canon closes — it would stay out of search and out of enrichment forever.
//
// Run inside the write transaction, the lookup cannot have its canon deleted underneath
// the marking. It does not stop a concurrent close (READ COMMITTED, no row lock), which
// is benign: the next recompute releases the row.
//
// A lookup failure is not fatal — it reports no canon, and the row is written unmarked
// exactly as before. Dedup is an improvement, never a condition of keeping the vacancy.
func CanonicalForRole(ctx context.Context, q *db.Queries, p db.UpsertJobParams, id int64) (db.CanonicalJobForRoleRow, bool) {
	if p.CompanySlug == "" || p.RoleFingerprint.String == "" {
		return db.CanonicalJobForRoleRow{}, false
	}
	row, err := q.CanonicalJobForRole(ctx, db.CanonicalJobForRoleParams{
		CompanySlug:     p.CompanySlug,
		RoleFingerprint: p.RoleFingerprint,
		Source:          p.Source,
		ExternalID:      p.ExternalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CanonicalJobForRoleRow{}, false
	}
	if err != nil {
		log.Printf("jobdedup: canonical posting for %s/%s: %v",
			p.CompanySlug, p.RoleFingerprint.String, err)
		return db.CanonicalJobForRoleRow{}, false
	}
	if row.ID >= id {
		return db.CanonicalJobForRoleRow{}, false
	}
	return row, true
}
