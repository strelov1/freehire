package cvedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/cv"
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

// refuseIfSanitizeDropsContent returns ErrListCap when the applied document already exceeds
// the per-role bullet ceiling. It reads ONE document, the pre-Sanitize one: comparing with the
// sanitized result by index is unsafe, since Sanitize also drops empty rows and shifts later
// roles. Dropping whitespace-only bullets is cleanup, not content loss.
func refuseIfSanitizeDropsContent(applied State) error {
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

// ListCapRemedy is the model-facing half of a list-cap refusal; the candidate-facing banner
// must not tell a person to "set an existing bullet", which is an instruction to a tool.
//
// Exported because the SPA strips exactly this sentence off the tool error
// (web/src/lib/assistant/bulletCapAlert.ts); TestListCapWireShape pins it.
const ListCapRemedy = "Set an existing bullet or remove one before inserting"

// ListCapError is a refused list-cap edit as DATA: which role is at the ceiling and what the
// ceiling was. Both audiences render from those fields (model: Error(), candidate:
// UserMessage()), so neither parses the other's prose.
type ListCapError struct {
	// Where names the role or project at the ceiling, as the candidate would recognise it
	// on their own CV ("Staff Engineer at Contoso", "project Atlas").
	Where string
	// Max is the ceiling in force when the edit was refused.
	Max int
}

// Error is the model-facing sentence: the refusal, the reason, and the remedy.
func (e *ListCapError) Error() string {
	return fmt.Sprintf("%s: %s: %s %s", ErrListCap, ListCapCode, e.reason(), ListCapRemedy)
}

// Unwrap makes errors.Is(err, ErrListCap) hold, which every caller already relies on.
func (e *ListCapError) Unwrap() error { return ErrListCap }

// UserMessage is the sentence shown to the candidate: no internal prefix, no instruction
// meant for a tool, and an explicit reassurance that nothing of theirs was lost.
func (e *ListCapError) UserMessage() string {
	return e.reason() + " Your existing bullets were kept."
}

// reason is the half both audiences share.
func (e *ListCapError) reason() string {
	return fmt.Sprintf("%s already has %d bullets (the maximum). "+
		"The edit was not applied and no existing bullets were deleted.", e.Where, e.Max)
}

func listCapErr(where string) error {
	return &ListCapError{Where: where, Max: cv.MaxBullets}
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

// UserListCapMessage turns a list-cap refusal into the sentence shown to the candidate, or ""
// when err is not one.
func UserListCapMessage(err error) string {
	var capErr *ListCapError
	if !errors.As(err, &capErr) {
		return ""
	}
	return capErr.UserMessage()
}
