// Package skillvec turns a set of canonical skill slugs into a fixed-width vector,
// so a vacancy's skills and a candidate's can be compared by cosine in the search
// engine rather than by a set operation over a window in application code.
//
// A skill's position is PERMANENT (see registry.go): shifting one corrupts every
// vector already stored in the search index, silently.
//
// Every recognised skill contributes the SAME amount. Weighting by rarity was tried and
// removed — it is incompatible with the ordering the feed promises. See AGENTS.md.
package skillvec

//go:generate go run ./gen

// Dimensions is the declared width of a skill vector — deliberately wider than the
// dictionary so newly mined skills get positions without a re-declaration.
//
// It is not free. Meilisearch stores the declared width whether or not the tail is
// occupied, so at the live catalogue's scale each 256 dimensions costs roughly
// 2.5 GB of index. Widening it later requires a full rebuild, and until that rebuild
// finishes the index rejects every document carrying the new width.
const Dimensions = 1024

// positions indexes the registry for lookup.
var positions = func() map[string]int {
	m := make(map[string]int, len(registry))
	for i, s := range registry {
		m[s] = i
	}
	return m
}()

// Position reports the permanent vector position of a canonical skill slug, and
// whether the slug has one. An unknown slug has none — dictionaries are dict-only,
// so an unrecognised skill contributes nothing rather than being hashed into an
// arbitrary slot.
func Position(skill string) (int, bool) {
	i, ok := positions[skill]
	return i, ok
}

// RegistrySize is how many positions are assigned. Dimensions minus this is the
// headroom left for dictionary growth before a rebuild-forcing re-declaration.
func RegistrySize() int { return len(registry) }
