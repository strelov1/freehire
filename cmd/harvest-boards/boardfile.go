package main

// entry is a probed-live board: the company it belongs to and the board id it crawls
// under — the two fields a catalog row needs from a harvest.
type entry struct {
	Company string
	Board   string
}

// mapSeeds converts raw seed tokens into canonical board ids using the prober's seedMapper,
// or returns them unchanged when the provider's token already is its board id.
func mapSeeds(p prober, seed []string) []string {
	m, ok := p.(seedMapper)
	if !ok {
		return seed
	}
	out := make([]string, len(seed))
	for i, s := range seed {
		out[i] = m.boardID(s)
	}
	return out
}

// newBoards returns the seed board ids not already present in existing, preserving seed
// order and de-duplicating within the seed. Comparison is by key(boardID): identity for most
// providers, case-folding for the ones whose board ids are case-insensitive (Workday, Ashby)
// so a lowercased harvest twin of a curated CamelCase board is dropped instead of crawled
// twice. The original (unkeyed) board id is what gets returned/stored.
func newBoards(seed []string, existing map[string]bool, key func(string) string) []string {
	seen := make(map[string]bool, len(existing)+len(seed))
	for b := range existing {
		seen[key(b)] = true
	}
	var out []string
	for _, s := range seed {
		k := key(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

// dedupKeyOf returns the board-id dedup key function for a prober: the prober's dedupKeyer
// if it has one, else identity (case-sensitive verbatim).
func dedupKeyOf(p prober) func(string) string {
	if k, ok := p.(dedupKeyer); ok {
		return k.dedupKey
	}
	return func(s string) string { return s }
}
