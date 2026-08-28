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

// ListCapRemedy is the model-facing half of a list-cap refusal: the correction the agent can
// act on inside the turn. It is separated from the candidate-facing half because the banner
// must not tell a person to "set an existing bullet" — that is an instruction to a tool.
//
// Exported because the SPA's own alert strips exactly this trailing sentence off the tool
// error (web/src/lib/assistant/bulletCapAlert.ts). TestListCapWireShape pins it, so rewording
// it here fails a Go test instead of silently degrading a banner in another language's build.
const ListCapRemedy = "Set an existing bullet or remove one before inserting"

// ListCapError is a refused list-cap edit as DATA: which role is at the ceiling and what the
// ceiling was. Both audiences are rendered FROM those fields — the model reads Error(), the
// candidate reads UserMessage() — so neither is recovered by cutting the other's prose.
//
// It used to be one English sentence that two other layers took apart with string surgery.
// Rewording it broke a banner and an SPA parser with no test failing anywhere.
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
// when err is not one. It reads the error's FIELDS — no cutting of its own prose, which is
// what used to make rewording the message a silent break.
func UserListCapMessage(err error) string {
	var capErr *ListCapError
	if !errors.As(err, &capErr) {
		return ""
	}
	return capErr.UserMessage()
}
