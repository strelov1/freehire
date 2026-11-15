// Package jobdedup answers one question for every write path that persists a posting:
// does the catalogue already hold this same vacancy under a different board identity?
//
// A company routinely posts one role once per city, so the copies share a RoleFingerprint
// (it excludes the location) and the batch recompute (db.RecomputeRoleDuplicatesForCompanies)
// collapses them to one canonical row. The batch runs every few hours; until then every copy
// is a separate searchable job. This package closes that window by answering the same
// question at write time, with the batch's answer (see CanonicalForRole).
package jobdedup

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// CanonicalForRole reports the open canonical posting of this row's role cluster, when
// one exists that is OLDER than the row just written (id).
//
// The age condition matters: RecomputeRoleDuplicatesForCompanies makes MIN(id) among a
// cluster's open rows the canon, so collapsing onto a NEWER posting would be inverted by
// the next reindex.
//
// Two inputs are refused here because marking either strands a row permanently:
//   - an empty fingerprint, which clusters with nobody;
//   - an empty company slug. The batch recompute skips those rows, so a row marked here
//     would never be released when its canon closes.
//
// Run inside the write transaction, the canon cannot be deleted under the marking. A
// concurrent close is not blocked (READ COMMITTED, no row lock), which is benign: the next
// recompute releases the row.
//
// A lookup failure is not fatal: it reports no canon and the row is written unmarked.
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
