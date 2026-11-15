package skilltag

import (
	"bufio"
	_ "embed"
	"fmt"
	"maps"
	"strings"
)

// Plain-language descriptions for the canonical skill slugs: what the thing IS, for a reader who
// does not already know. The slug is what the parser resolves to, the label is how it is written
// (labels.go), the description is what it means; none can be derived from the others.
//
// The text is a static, reviewed artifact: cmd/gen-skill-descriptions drafts a wave with an LLM
// and PRINTS it; a human edits and commits. Nothing is produced at request time. It lives in a
// TSV rather than a Go map because each value is a sentence, and one unquoted line per skill is
// what a reviewer can read.

//go:embed descriptions.tsv
var descriptionsTSV string

var descriptions = mustLoadDescriptions(descriptionsTSV)

// Description is the curated sentence for a canonical skill, or "" when no wave has described it
// yet or the slug is not a skill. Both answer alike on purpose: surfaces render a described skill
// differently (tooltip, glossary page), so the absence must be a testable value, never a
// placeholder that could reach a reader.
func Description(canonical string) string {
	return descriptions[canonical]
}

// Descriptions is every described canonical, for the catalog cmd/gen-contracts emits and the
// glossary index. A copy: callers must not be able to edit the process-wide dictionary.
func Descriptions() map[string]string {
	return maps.Clone(descriptions)
}

// loadDescriptions parses the embedded TSV, rejecting anything malformed rather than skipping
// it: the file is hand-written here, so a bad row is a mistake in this repository.
func loadDescriptions(tsv string) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(tsv))
	for line := 1; sc.Scan(); line++ {
		row := strings.TrimSuffix(sc.Text(), "\r") // tolerate a CRLF checkout
		// The only two shapes skipped rather than judged. No canonical begins with '#' (the "C#" skill
		// is slugged "csharp"), so a comment cannot swallow a real row.
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		slug, desc, ok := strings.Cut(row, "\t")
		switch {
		case !ok:
			return nil, fmt.Errorf("line %d: no tab separating slug from description", line)
		case slug == "":
			return nil, fmt.Errorf("line %d: blank slug", line)
		// Symmetric with the description's trim check below: a stray leading space would parse into the
		// key " dbt" and only the orphan test would notice, far from the cause.
		case strings.TrimSpace(slug) != slug:
			return nil, fmt.Errorf("line %d: slug %q has surrounding space", line, slug)
		case slug != strings.ToLower(slug):
			return nil, fmt.Errorf("line %d: slug %q is not lowercase", line, slug)
		case desc == "":
			return nil, fmt.Errorf("line %d: %q has a blank description", line, slug)
		case strings.Contains(desc, "\t"):
			return nil, fmt.Errorf("line %d: %q has a tab inside its description", line, slug)
		case strings.TrimSpace(desc) != desc:
			return nil, fmt.Errorf("line %d: %q has surrounding space in its description", line, slug)
		}
		if _, seen := out[slug]; seen {
			return nil, fmt.Errorf("line %d: %q is described twice", line, slug)
		}
		out[slug] = desc
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading descriptions: %w", err)
	}
	return out, nil
}

// mustLoadDescriptions panics on a malformed file. The input is embedded at build time and the
// package's tests parse it, so this fires only on a file that never passed them.
func mustLoadDescriptions(tsv string) map[string]string {
	out, err := loadDescriptions(tsv)
	if err != nil {
		panic("skilltag: descriptions.tsv: " + err.Error())
	}
	return out
}
