package cvedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

// projectNamePath and projectWholePath address what an op's VALUE means for this check: the
// op's own path, not its position in the final document — see refuseIfProjectLooksLikeJob.
var (
	projectNamePath  = regexp.MustCompile(`^projects\[\d+\]\.name$`)
	projectWholePath = regexp.MustCompile(`^projects\[\d+\]$`)
)

// refuseIfProjectLooksLikeJob returns ErrProjectLooksLikeJob when a batch's operations write a
// projects[] name carrying a job's date span.
//
// It reads each op's OWN Value, not the final document. A path's index is only correct against
// the state that op was applied against — a later insert or remove on the same list shifts
// every index after it, so re-deriving "what index 0 ends up holding" from the final state can
// point at a different entry than the one this op actually wrote. Reading the op's own value
// sidesteps that correspondence entirely: it is exactly what this op is trying to write,
// regardless of where the list repositions it afterward.
func refuseIfProjectLooksLikeJob(ops []Op) error {
	for _, op := range ops {
		if op.Kind != OpSet && op.Kind != OpInsert {
			continue
		}
		name, ok := projectNameWritten(op)
		if !ok || name == "" {
			continue
		}
		if dateRangeInName.MatchString(name) {
			return fmt.Errorf("%w: %q. Job roles with a start and end date belong under "+
				"experience[], not projects[]", ErrProjectLooksLikeJob, name)
		}
	}
	return nil
}

// projectNameWritten reads the name an operation sets on a projects[] entry, from the op's own
// path and value — "projects[i].name" (value is the name itself) or "projects[i]" (value is a
// whole project; its name, if any, is pulled out via the same json tag cv.Project uses). Any
// other path under projects[] (bullets, link) is not a name write and returns ok=false.
func projectNameWritten(op Op) (string, bool) {
	path := string(op.Path)
	switch {
	case projectNamePath.MatchString(path):
		name, _ := op.Value.(string)
		return name, true
	case projectWholePath.MatchString(path):
		var whole struct {
			Name string `json:"name"`
		}
		blob, err := json.Marshal(op.Value)
		if err != nil {
			return "", false
		}
		if err := json.NewDecoder(bytes.NewReader(blob)).Decode(&whole); err != nil {
			return "", false
		}
		return whole.Name, true
	default:
		return "", false
	}
}
