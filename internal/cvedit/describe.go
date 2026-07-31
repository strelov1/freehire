package cvedit

import (
	"fmt"
	"strings"
)

// Describe renders a batch as the line the history feed shows.
//
// It is generated from the operations, never supplied by the model. The description is
// rendered in the workspace as the application's own words, and model-authored text
// presented that way is exactly what the assistant's untrusted-content boundary exists to
// keep out. An agent's own reason for an edit travels separately, as a note, attributed to
// it.
//
// The document is passed in so an address can be named the way the candidate sees it:
// `experience[2].bullets[1]` is "a bullet in Senior Engineer, Acme", not a path.
func Describe(ops []Op, before, after State) string {
	switch len(ops) {
	case 0:
		return "No change"
	case 1:
		return describeOne(ops[0], before, after)
	}

	// Several operations in one place read better as a count of that place than as a list.
	if where, ok := commonPlace(ops, after); ok {
		return fmt.Sprintf("Edited %d things in %s", len(ops), where)
	}
	return fmt.Sprintf("Edited %d places", len(ops))
}

func describeOne(op Op, before, after State) string {
	shape := shapeOf(op.Path)
	verb := map[OpKind]string{OpSet: "Rewrote", OpInsert: "Added", OpRemove: "Removed", OpMove: "Moved"}[op.Kind]

	switch shape {
	case "title":
		return "Renamed the CV"
	case "template_id":
		if s, ok := op.Value.(string); ok {
			return fmt.Sprintf("Switched to the %s template", s)
		}
		return "Switched template"
	case "summary":
		return "Rewrote the summary"
	case "style.font_family", "style.font_size", "style.line_height":
		return "Changed the typography"
	case "margins.top", "margins.right", "margins.bottom", "margins.left":
		return "Changed the page margins"
	case "experience[].bullets[]":
		return fmt.Sprintf("%s a bullet in %s", verb, entryName(op.Path, before, after))
	case "experience[].stack[]", "experience[].stack":
		return fmt.Sprintf("Changed the technologies in %s", entryName(op.Path, before, after))
	case "experience[].summary":
		return fmt.Sprintf("Rewrote the description of %s", entryName(op.Path, before, after))
	case "experience[]":
		return fmt.Sprintf("%s an experience entry", verb)
	case "skills[].items[]", "skills[].items":
		return "Changed the skills"
	case "skills[]":
		return fmt.Sprintf("%s a skill group", verb)
	case "projects[].bullets[]", "projects[]":
		return fmt.Sprintf("%s a project", verb)
	case "education[]":
		return fmt.Sprintf("%s an education entry", verb)
	case "certifications[]":
		return fmt.Sprintf("%s a certification", verb)
	case "languages[]":
		return fmt.Sprintf("%s a language", verb)
	}

	// Anything the list above has not been taught to name yet still reads as a sentence
	// rather than as a path, and adding a field to the document cannot produce a blank line.
	return fmt.Sprintf("%s %s", verb, humanise(shape))
}

// entryName names an experience entry the way it appears on the page. It reads the state
// AFTER the change where it can, so a removed entry is still named from the state before.
func entryName(p Path, before, after State) string {
	steps := p.steps()
	if len(steps) < 2 || !steps[1].isIndex() {
		return "an experience entry"
	}
	at := steps[1].index

	for _, state := range []State{after, before} {
		if at < len(state.Experience) {
			e := state.Experience[at]
			switch {
			case e.Role != "" && e.Company != "":
				return e.Role + ", " + e.Company
			case e.Company != "":
				return e.Company
			case e.Role != "":
				return e.Role
			}
		}
	}
	return "an experience entry"
}

// commonPlace reports the entry every operation in a batch sits in, when there is one.
func commonPlace(ops []Op, after State) (string, bool) {
	first := entryPrefix(ops[0].Path)
	if first == "" {
		return "", false
	}
	for _, op := range ops[1:] {
		if entryPrefix(op.Path) != first {
			return "", false
		}
	}
	return entryName(ops[0].Path, after, after), true
}

// entryPrefix is the `experience[n]` an address sits under, or empty for anything else.
func entryPrefix(p Path) string {
	steps := p.steps()
	if len(steps) < 2 || steps[0].name != "experience" || !steps[1].isIndex() {
		return ""
	}
	return fmt.Sprintf("experience[%d]", steps[1].index)
}

// humanise turns a path shape into something readable: `certifications[].issuer` becomes
// "the issuer of a certification".
func humanise(shape string) string {
	parts := strings.Split(strings.ReplaceAll(shape, "[]", ""), ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return "the " + strings.ReplaceAll(strings.Join(parts, " of the "), "_", " ")
}
