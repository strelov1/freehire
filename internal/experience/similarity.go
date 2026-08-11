package experience

import (
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// softDupThreshold is the Jaccard similarity on claim meaningful-tokens at which
// two atoms are treated as soft-duplicates. Tuned so the motivating near-paraphrase
// pair (two assistant readings of one Chromium/faster-whisper plugin, ~0.44) clusters,
// while distinct claims that differ in numbers (~0.33) or share mostly stopwords (~0.25)
// do not. Merge still requires an explicit owner confirm.
const softDupThreshold = 0.40

// digitRE matches any Unicode digit — a claim that already carries a number is
// not thin on metrics even when the metrics array is empty.
var digitRE = regexp.MustCompile(`\d`)

// Richness reports whether an atom is thin on situation or numbers. Derived on
// read; never persisted.
func Richness(a Atom) (needsContext, needsMetrics bool) {
	needsContext = strings.TrimSpace(a.Context) == ""
	needsMetrics = len(a.Metrics) == 0 && !digitRE.MatchString(a.Claim)
	return needsContext, needsMetrics
}

// SoftDuplicateClusters groups an owner's atoms into near-paraphrase clusters
// within one employment bucket (each employment_id, plus one bucket for
// unplaced). Claim-only token Jaccard ≥ softDupThreshold. Clusters of size 1
// are omitted. Ids within a cluster are sorted; clusters follow the earliest
// member's position in the input.
func SoftDuplicateClusters(atoms []Atom) [][]uuid.UUID {
	if len(atoms) < 2 {
		return nil
	}

	type bucketKey string
	buckets := map[bucketKey][]int{}
	for i, a := range atoms {
		var key bucketKey
		if a.EmploymentID != nil {
			key = bucketKey(a.EmploymentID.String())
		}
		buckets[key] = append(buckets[key], i)
	}

	type cluster struct {
		ids       []uuid.UUID
		firstSeen int
	}
	var clusters []cluster

	for _, idxs := range buckets {
		if len(idxs) < 2 {
			continue
		}
		parent := make([]int, len(idxs))
		for i := range parent {
			parent[i] = i
		}
		var find func(int) int
		find = func(x int) int {
			if parent[x] != x {
				parent[x] = find(parent[x])
			}
			return parent[x]
		}
		union := func(a, b int) {
			ra, rb := find(a), find(b)
			if ra != rb {
				parent[ra] = rb
			}
		}

		tokens := make([]map[string]bool, len(idxs))
		for i, atomIdx := range idxs {
			tokens[i] = newSet(meaningfulTokens(atoms[atomIdx].Claim))
		}
		for i := 0; i < len(idxs); i++ {
			for j := i + 1; j < len(idxs); j++ {
				if jaccard(tokens[i], tokens[j]) >= softDupThreshold {
					union(i, j)
				}
			}
		}

		groups := map[int][]int{}
		for i := range idxs {
			r := find(i)
			groups[r] = append(groups[r], i)
		}
		for _, members := range groups {
			if len(members) < 2 {
				continue
			}
			ids := make([]uuid.UUID, 0, len(members))
			firstSeen := idxs[members[0]]
			for _, m := range members {
				atomIdx := idxs[m]
				ids = append(ids, atoms[atomIdx].ID)
				if atomIdx < firstSeen {
					firstSeen = atomIdx
				}
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
			clusters = append(clusters, cluster{ids: ids, firstSeen: firstSeen})
		}
	}

	sort.Slice(clusters, func(i, j int) bool { return clusters[i].firstSeen < clusters[j].firstSeen })
	out := make([][]uuid.UUID, len(clusters))
	for i, c := range clusters {
		out[i] = c.ids
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for t := range a {
		if b[t] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
