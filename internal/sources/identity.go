package sources

import "fmt"

// NamespaceExternalID is the single definition of the dedup-key external_id format shared by
// the ingest pipeline and the link-source adapters: the platform's native posting id
// namespaced by board, so two companies on one multi-tenant platform cannot collide. A
// boardless source passes an empty board, yielding a ":<id>" key. Both write paths to
// UpsertJob — the ingest pipeline (via jobIdentity) and the link-source adapters (via
// tg-extract's CompleteLinks) — MUST route their external_id through here, so a job resolved
// from a Telegram link dedups against the same posting crawled from a board file rather than
// creating a duplicate row.
func NamespaceExternalID(board, externalID string) string {
	return fmt.Sprintf("%s:%s", board, externalID)
}
