package cvedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/cv"
)

// ErrListCap is returned when Apply would leave more than cv.MaxBullets
// non-empty bullets on one experience or project — Sanitize would keep the
// first MaxBullets and drop the rest. The edit is refused so trailing content
// cannot vanish while History still claims an "add".
var ErrListCap = errors.New("cvedit: this edit would drop existing CV content")

// ListCapCode is a stable token in ErrListCap messages so the SPA can surface a
// candidate-facing alert without scraping free-form text. The model still reads
// the full sentence.
const ListCapCode = "bullet_cap"

// refuseIfSanitizeDropsContent returns ErrListCap when the applied document
// already exceeds the per-role bullet ceiling. Comparing applied vs after by
// index is unsafe: Sanitize also drops empty experience/project rows, which
// shifts later roles and looks like bullet loss.
// Dropping whitespace-only bullets is allowed — that is cleanup, not content loss.
func refuseIfSanitizeDropsContent(applied, _ State) error {
	for i, e := range applied.Experience {
		if countNonEmpty(e.Bullets) > cv.MaxBullets {
			return listCapErr(experienceLabel(e, i))
		}
	}
	for i, p := range applied.Projects {
		if countNonEmpty(p.Bullets) > cv.MaxBullets {
			return listCapErr(projectLabel(p, i))
		}
	}
	return nil
}

func listCapErr(where string) error {
	return fmt.Errorf("%w: %s: %s already has %d bullets (the maximum). "+
		"The edit was not applied and no existing bullets were deleted. "+
		"Set an existing bullet or remove one before inserting",
		ErrListCap, ListCapCode, where, cv.MaxBullets)
}

func countNonEmpty(bullets []string) int {
	n := 0
	for _, b := range bullets {
		if strings.TrimSpace(b) != "" {
			n++
		}
	}
	return n
}

func experienceLabel(e cv.ExperienceItem, i int) string {
	role := strings.TrimSpace(e.Role)
	company := strings.TrimSpace(e.Company)
	switch {
	case role != "" && company != "":
		return role + " at " + company
	case role != "":
		return role
	case company != "":
		return company
	default:
		return fmt.Sprintf("experience[%d]", i)
	}
}

func projectLabel(p cv.Project, i int) string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return "project " + name
	}
	return fmt.Sprintf("projects[%d]", i)
}

// UserListCapMessage turns an ErrListCap into the sentence shown to the candidate.
// Internal prefixes and model-facing remedy stay out of the banner.
func UserListCapMessage(err error) string {
	if err == nil || !errors.Is(err, ErrListCap) {
		return ""
	}
	msg := err.Error()
	if _, rest, ok := strings.Cut(msg, ListCapCode+": "); ok {
		msg = rest
	}
	if before, _, ok := strings.Cut(msg, ". Set an existing"); ok {
		return before + ". Your existing bullets were kept."
	}
	return msg
}
