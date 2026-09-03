package boardcatalog

import "strings"

// PlaceholderCompany derives a display-name placeholder from a board slug when the real
// company name is not known at insert time — a crowdsourced contribution recognizes
// (provider, board) from a URL alone (internal/ingest/atsboard, deliberately
// network-free), with no company name available, but boards.company is required because
// board-based adapters (greenhouse, lever, ashby) write it verbatim as every crawled
// job's employer name. A curator corrects it via cmd/add-board --rename once they see the
// pending board and know the real name.
func PlaceholderCompany(board string) string {
	words := strings.FieldsFunc(board, func(r rune) bool { return r == '-' || r == '_' })
	if len(words) == 0 {
		return board
	}
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
