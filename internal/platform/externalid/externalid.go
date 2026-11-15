// Package externalid defines the format of jobs.external_id, the board-namespaced posting key
// that with source is the catalogue's dedup key, and the LIKE pattern selecting one board's keys.
// It sits beside the storage because both are properties of the column; the escaping exists for
// the (source, external_id text_pattern_ops) index. internal/platform/db's integration test
// asserts the two agree.
package externalid

import (
	"fmt"
	"strings"
)

// Namespace is the single definition of the external_id format: the platform's posting id
// namespaced by board, so two companies on one platform cannot collide. A boardless source
// passes an empty board (":<id>"). Both write paths to UpsertJob, the ingest pipeline (via
// jobIdentity) and the link-source adapters (via tg-extract's CompleteLinks), MUST route through
// here so a Telegram-linked job dedups against the same posting crawled from a board.
func Namespace(board, externalID string) string {
	return fmt.Sprintf("%s:%s", board, externalID)
}

// likeSpecial escapes the three characters LIKE reads as syntax, backslash first so the escapes
// it introduces are not escaped again.
var likeSpecial = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikePrefix is the LIKE pattern matching every string that starts with prefix, escaped the same
// way as BoardPattern; used as the board-scoped close's url predicate for a region-scoped board.
func LikePrefix(prefix string) string {
	return likeSpecial.Replace(prefix) + "%"
}

// BoardPattern is the LIKE pattern matching every external_id Namespace would produce for a
// board, and nothing else. It rides the (source, external_id text_pattern_ops) index.
//
// The board name is escaped because an underscore (Capital_One) would match any character and
// claim a sibling board's postings.
func BoardPattern(board string) string {
	return likeSpecial.Replace(board) + ":%"
}
