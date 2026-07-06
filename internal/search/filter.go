package search

import (
	"strconv"
	"strings"
)

// Filter expression helpers. These build Meilisearch filter fragments so the
// handler can express facet intent without knowing Meilisearch syntax, and so
// untrusted query-param values are always escaped at one place (see Eq).

// Eq builds an equality fragment `attr = "value"` with the value quoted and
// escaped. Quoting is mandatory: an unescaped value could otherwise inject
// filter logic (e.g. `senior" OR work_mode = "remote`).
func Eq(attr, value string) string {
	return attr + " = " + quote(value)
}

// Neq builds an inequality fragment `attr != "value"` (escaped), used by the
// exclude facets to filter a value out.
func Neq(attr, value string) string {
	return attr + " != " + quote(value)
}

// EqBool builds an equality fragment against a boolean attribute (unquoted, as
// Meilisearch compares booleans literally).
func EqBool(attr string, v bool) string {
	return attr + " = " + strconv.FormatBool(v)
}

// IsEmpty builds an `attr IS EMPTY` fragment, matching documents whose array (or
// string) attribute is empty. It backs the regions "not specified" sentinel —
// selecting jobs whose geography did not resolve — without a materialized column.
func IsEmpty(attr string) string {
	return attr + " IS EMPTY"
}

// IsNotEmpty builds `attr IS NOT EMPTY`, the exclude form of IsEmpty (jobs that DO
// carry a value for the attribute).
func IsNotEmpty(attr string) string {
	return attr + " IS NOT EMPTY"
}

// Gte builds a `attr >= n` numeric fragment.
func Gte(attr string, n int) string {
	return attr + " >= " + strconv.Itoa(n)
}

// Lte builds a `attr <= n` numeric fragment.
func Lte(attr string, n int) string {
	return attr + " <= " + strconv.Itoa(n)
}

// NotIn builds an `attr NOT IN [a, b, c]` fragment excluding a set of numeric
// ids (Meilisearch's native list-exclusion operator). It is used by the swipe
// deck to drop the caller's already-judged jobs by id. An empty set yields the
// empty string, so the caller adds no filter fragment at all (Meilisearch has no
// meaningful "NOT IN []").
func NotIn(attr string, ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(attr)
	b.WriteString(" NOT IN [")
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	b.WriteByte(']')
	return b.String()
}

// Filter nests OR-groups into a single AND filter for Meilisearch: fragments
// within a group are ORed, groups are ANDed. Empty groups are dropped; the
// result is nil when nothing remains, which Meilisearch treats as "no filter".
func Filter(groups ...[]string) any {
	out := make([][]string, 0, len(groups))
	for _, g := range groups {
		if len(g) > 0 {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AndNotSkills narrows a base filter (as built by FilterFromValues, either a
// [][]string or nil) to documents whose `skills` array contains none of the given
// skills — one `skills != "<skill>"` AND group per skill, since array
// non-membership is per-value. It backs the verdict's "uncovered vacancies" query
// (a role's jobs listing none of the candidate's skills). Empty skills leave the
// base unchanged; a nil base with no skills stays nil (no filter).
func AndNotSkills(base any, skills []string) any {
	groups, _ := base.([][]string)
	for _, s := range skills {
		if s != "" {
			groups = append(groups, []string{Neq("skills", s)})
		}
	}
	return Filter(groups...)
}

// AndSkillsPresent narrows a base filter (as built by FilterFromValues) to
// documents whose `skills` array is non-empty. It backs the verdict's
// skill-bearing count: skill frequency is measured against vacancies that list
// at least one tagged skill, not the whole role, so postings the tagger left
// skill-less don't deflate every frequency.
func AndSkillsPresent(base any) any {
	groups, _ := base.([][]string)
	groups = append(groups, []string{IsNotEmpty("skills")})
	return Filter(groups...)
}

// quote wraps a value in a Meilisearch string literal, backslash-escaping the
// double-quote and backslash characters.
func quote(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}
