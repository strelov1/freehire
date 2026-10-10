package cvedit

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// ErrProjectLooksLikeJob is returned when an agent batch writes a projects[] entry whose name
// carries a "<start> - <end>" date span — the one shape a real portfolio project's name
// essentially never has, and a job's tenure essentially always does. cv.Project has no company
// or date fields at all, so a job filed there loses its tenure outright rather than merely
// sitting under the wrong heading; the edit is refused so the agent redoes it as experience[]
// instead of losing that structure silently.
var ErrProjectLooksLikeJob = errors.New("cvedit: this project's name reads like a job's tenure")

// dateRangeInName matches a year-to-year(-ish) span such as "2020 - 2023", "2020–Present", or
// "2019 to current".
var dateRangeInName = regexp.MustCompile(`(?i)(19|20)\d{2}\s*(-|–|—|to)\s*((19|20)\d{2}|present|current)`)

// projectPathIndex extracts the index from a path addressing a projects[] entry or something
// inside one, e.g. "projects[2]" or "projects[2].name".
var projectPathIndex = regexp.MustCompile(`^projects\[(\d+)\]`)

// refuseIfProjectLooksLikeJob returns ErrProjectLooksLikeJob when a batch writes a projects[]
// entry whose resulting name carries a job's date span. Only name is checked — a project's
// bullets legitimately mention a year without the entry being misfiled — and only operations
// that assert new content (set, insert) are considered, matching requireEvidence's own
// distinction between asserting and merely rearranging.
func refuseIfProjectLooksLikeJob(ops []Op, applied State) error {
	for _, op := range ops {
		if op.Kind != OpSet && op.Kind != OpInsert {
			continue
		}
		m := projectPathIndex.FindStringSubmatch(string(op.Path))
		if m == nil {
			continue
		}
		i, err := strconv.Atoi(m[1])
		if err != nil || i >= len(applied.Projects) {
			continue
		}
		if dateRangeInName.MatchString(applied.Projects[i].Name) {
			return fmt.Errorf("%w: %s. Job roles with a start and end date belong under "+
				"experience[], not projects[]", ErrProjectLooksLikeJob, projectLabel(applied.Projects[i], i))
		}
	}
	return nil
}
