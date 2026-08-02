// Package userjob holds the per-user job-tracking vocabulary shared by the HTTP
// handler and the contract generator.
package userjob

// Stages is the ordered application-stage vocabulary (active stages then terminal)
// and the single source of truth: the track endpoint validates against it and the
// generated frontend list is emitted from it.
var Stages = []string{
	"applied", "screening", "responded", "interview",
	"offer", "accepted", "rejected", "withdrawn", "expired",
}

// ValidStage reports whether s is a known application stage.
func ValidStage(s string) bool {
	for _, st := range Stages {
		if st == s {
			return true
		}
	}
	return false
}
