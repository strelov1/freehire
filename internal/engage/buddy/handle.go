package buddy

import (
	"fmt"
	"math/rand/v2"
)

// The buddy handle has the shape of the community persona ("quiet-otter-42") but is
// minted independently, so no two of a person's pseudonyms can be joined. The banks
// differ from the community's for the same reason.
var handleAdjectives = []string{
	"amber", "brisk", "candid", "dusty", "earnest", "fuzzy", "golden", "hardy",
	"icy", "jolly", "kind", "lively", "misty", "neat", "olive", "proud",
	"rustic", "snug", "tidy", "vivid",
}

var handleNouns = []string{
	"alder", "bison", "crane", "dune", "egret", "fjord", "gecko", "heron",
	"iris", "juniper", "kestrel", "lagoon", "moth", "newt", "osprey", "pebble",
	"quail", "reef", "sparrow", "tundra",
}

// GenerateHandle returns a fresh handle like "amber-heron-42" or "amber-heron-4217". The
// suffix runs to four digits so the space (|adj| x |noun| x 9990, about four million)
// stays far above any membership the retry has to work against. UNIQUE(handle) plus the
// service's retry turns "usually unique" into a guarantee.
func GenerateHandle() string {
	adj := handleAdjectives[rand.IntN(len(handleAdjectives))]
	noun := handleNouns[rand.IntN(len(handleNouns))]
	return fmt.Sprintf("%s-%s-%d", adj, noun, rand.IntN(9990)+10)
}
