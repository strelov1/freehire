// Package externalid defines the format of jobs.external_id — the board-namespaced
// posting key that, with source, is the catalogue's dedup key — and the LIKE pattern that
// selects one board's keys out of it.
//
// It sits beside the storage rather than with the crawlers because both halves are
// properties of the stored column: the namespacing is the column's format, and the
// escaping exists for the (source, external_id text_pattern_ops) index the board-scoped
// query rides. internal/platform/db's integration test asserts the two agree.
package externalid

import (
	"fmt"
	"strings"
)

// Namespace is the single definition of the dedup-key external_id format shared by
// the ingest pipeline and the link-source adapters: the platform's native posting id
// namespaced by board, so two companies on one multi-tenant platform cannot collide. A
// boardless source passes an empty board, yielding a ":<id>" key. Both write paths to
// UpsertJob — the ingest pipeline (via jobIdentity) and the link-source adapters (via
// tg-extract's CompleteLinks) — MUST route their external_id through here, so a job resolved
// from a Telegram link dedups against the same posting crawled from a board file rather than
// creating a duplicate row.
func Namespace(board, externalID string) string {
	return fmt.Sprintf("%s:%s", board, externalID)
}

// likeSpecial escapes the three characters LIKE reads as syntax, backslash first so the escapes
// it introduces are not escaped again.
var likeSpecial = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// BoardPattern is the LIKE pattern matching every external_id Namespace would produce
// for a board — the board's namespace and nothing else. It is the seen-set's board scope: the
// query behind it rides the (source, external_id text_pattern_ops) index, so a board's stored ids
// are read without scanning the provider's whole catalogue.
//
// The board name is escaped because LIKE would otherwise read parts of it as syntax: a third of
// the workday board names carry an underscore (Capital_One), which matches any one character, so
// an unescaped pattern would claim a sibling board's postings as this board's own.
// LikePrefix is the LIKE pattern matching every string that starts with prefix, escaped the same
// way — the board-scoped close's url predicate for a region-scoped board.
func LikePrefix(prefix string) string {
	return likeSpecial.Replace(prefix) + "%"
}

func BoardPattern(board string) string {
	return likeSpecial.Replace(board) + ":%"
}
