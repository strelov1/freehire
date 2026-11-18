package main

import (
	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/db"
)

type remoteStratum struct {
	global  bool
	budget  int32
	weights []qualityaudit.SourceWeight
}

// A random remote sample almost never encounters global tags when those tags are rare.
// Reserve half of the existing judge budget for them, with disjoint remote-only and global
// strata so no posting is judged twice in a run. These are per-stratum measurements, not an
// estimate of the prevalence of incorrect tags across the whole catalogue.
func remoteStrata(counts []db.CountOpenRemoteJobsBySourceRow, budget int32) []remoteStratum {
	global, other := remoteStratum{global: true}, remoteStratum{}
	for _, c := range counts {
		if c.GlobalJobs > 0 {
			global.weights = append(global.weights, qualityaudit.SourceWeight{Source: c.Source, Weight: c.GlobalJobs})
		}
		if n := c.RemoteJobs - c.GlobalJobs; n > 0 && c.RemoteJobs >= minRemote {
			other.weights = append(other.weights, qualityaudit.SourceWeight{Source: c.Source, Weight: n})
		}
	}
	if len(global.weights) == 0 {
		other.budget = budget
		return []remoteStratum{other}
	}
	if len(other.weights) == 0 {
		global.budget = budget
		return []remoteStratum{global}
	}
	global.budget = budget/2 + budget%2
	other.budget = budget - global.budget
	return []remoteStratum{global, other}
}
