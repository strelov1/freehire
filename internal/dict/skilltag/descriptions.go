package skilltag

import (
	"bufio"
	_ "embed"
	"fmt"
	"maps"
	"strings"
)

// Plain-language descriptions for the canonical skill slugs — what the thing IS, for a
// reader who does not already know.
//
// This is the vocabulary's third side. The slug is what the parser resolves to
// (dictionaries.go), the label is how it is written for a reader (labels.go), and the
// description is what it means. None of the three can be derived from the others: no
// amount of casing turns "dbt" into "a SQL transformation tool for data warehouses".
//
// The text is a static, reviewed artifact. cmd/gen-skill-descriptions drafts a wave with
// an LLM and PRINTS it; a human edits and commits. Nothing here is produced at request
// time, which is the same rule every other dictionary in internal/dict keeps.
//
// It lives in a TSV rather than a Go map because the vocabulary is hundreds of entries
// whose value is a sentence: one unquoted line per skill is what a reviewer can actually
// read a wave of. internal/dict/location ships its largest dictionary the same way.

// describedFloor is how many canonicals carry a description today. It is a ratchet: a
// wave raises it, and nothing may lower it. Coverage cannot be "all of them" yet — the
// whole vocabulary reviewed in one pull request is a review nobody performs — so the
// rule that CAN hold from day one is that coverage never goes backwards.
//
// When this reaches len(Canonicals()) it is deleted, and the coverage test becomes the
// absolute rule the labels already carry: a canonical with no description fails the
// build. One rule, not two that can disagree.
const describedFloor = 487

//go:embed descriptions.tsv
var descriptionsTSV string

var descriptions = mustLoadDescriptions(descriptionsTSV)

// Description is the curated sentence for a canonical skill, or "" when no wave has
// described it yet and for a slug that is not a skill at all.
//
// The two answer alike on purpose. Every surface renders a described skill differently
// from an undescribed one — no tooltip, no glossary page — so the absence has to be a
// value a caller can test, never a placeholder that could reach a reader.
func Description(canonical string) string {
	return descriptions[canonical]
}

// Descriptions is every described canonical, for the catalog cmd/gen-contracts emits
// and for the glossary's own index. A copy: the shipped dictionary is process-wide and
// a caller that ranges over it must not be able to edit it for everyone else.
func Descriptions() map[string]string {
	return maps.Clone(descriptions)
}

// loadDescriptions parses the embedded TSV, rejecting anything malformed rather than
// skipping it. The file is hand-written here, so a bad row is a mistake in this
// repository and not noise in someone else's dataset — the opposite of
// internal/dict/location, which tolerates what GeoNames hands it.
func loadDescriptions(tsv string) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(tsv))
	for line := 1; sc.Scan(); line++ {
		row := strings.TrimSuffix(sc.Text(), "\r") // tolerate a CRLF checkout
		// The only two shapes skipped rather than judged. No canonical begins with '#'
		// — the one skill whose name carries it is slugged "csharp" — so a comment
		// cannot swallow a real row.
		if row == "" || strings.HasPrefix(row, "#") {
			continue
		}
		slug, desc, ok := strings.Cut(row, "\t")
		switch {
		case !ok:
			return nil, fmt.Errorf("line %d: no tab separating slug from description", line)
		case slug == "":
			return nil, fmt.Errorf("line %d: blank slug", line)
		// Symmetric with the description's own trim check below. Without it a row with
		// a stray leading space parses into the key " dbt" and only the orphan test
		// notices, three files away and with a message about a skill that does not exist.
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

// mustLoadDescriptions panics on a malformed file. The input is embedded at build time
// and the package's tests parse it, so this can only fire on a file that never passed
// them — a state worth failing loudly rather than serving a dictionary with a hole.
func mustLoadDescriptions(tsv string) map[string]string {
	out, err := loadDescriptions(tsv)
	if err != nil {
		panic("skilltag: descriptions.tsv: " + err.Error())
	}
	return out
}
