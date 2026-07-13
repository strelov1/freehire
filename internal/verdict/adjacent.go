package verdict

import "github.com/strelov1/freehire/internal/skilladjacency"

// adjacentHeld returns the first neighbour of `roleSkill` that the CV holds (in
// declared or body), or "" when none — i.e. the close skill to reframe around. The
// declared/body split collapses into one held-set for the shared adjacency lookup
// (skilladjacency.AdjacentVia), which preserves the listed-neighbour order.
func adjacentHeld(roleSkill string, declared, body map[string]bool) string {
	held := make(map[string]bool, len(declared)+len(body))
	for s := range declared {
		held[s] = true
	}
	for s := range body {
		held[s] = true
	}
	via, _ := skilladjacency.AdjacentVia(roleSkill, held)
	return via
}
