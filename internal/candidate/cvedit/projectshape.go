package cvedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// ErrProjectLooksLikeJob is returned when an agent batch writes a projects[] entry whose name
// carries a "<start> - <end>" date span, which a job's tenure has and a project name almost
// never does. cv.Project has no company or date fields, so a job filed there would lose its
// tenure; the edit is refused so the agent redoes it as experience[].
var ErrProjectLooksLikeJob = errors.New("cvedit: this project's name reads like a job's tenure")

// dateRangeInName matches a year-to-year(-ish) span such as "2020 - 2023", "2020–Present", or
// "2019 to current".
var dateRangeInName = regexp.MustCompile(`(?i)(19|20)\d{2}\s*(-|–|—|to)\s*((19|20)\d{2}|present|current)`)

// projectNamePath and projectWholePath match an op's own path, not its position in the final
// document; see refuseIfProjectLooksLikeJob.
var (
	projectNamePath  = regexp.MustCompile(`^projects\[\d+\]\.name$`)
	projectWholePath = regexp.MustCompile(`^projects\[\d+\]$`)
)

// refuseIfProjectLooksLikeJob returns ErrProjectLooksLikeJob when a batch's operations write a
// projects[] name carrying a job's date span.
//
// It reads each op's OWN Value, not the final document: a later insert or remove on the same
// list shifts indices, so the final state can point at a different entry than this op wrote.
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
// path and value: "projects[i].name" (the value is the name) or "projects[i]" (a whole project,
// decoded via cv.Project's json tag). Any other path under projects[] returns ok=false.
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
