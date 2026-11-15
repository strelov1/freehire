package search

import (
	"strconv"
	"strings"
)

// Filter expression helpers. They build Meilisearch filter fragments and escape
// untrusted values in one place (see quote).

// Eq builds `attr = "value"`, quoted and escaped. Quoting is mandatory: an unescaped value
// could inject filter logic (`senior" OR work_mode = "remote`).
func Eq(attr, value string) string {
	return attr + " = " + quote(value)
}

// Neq builds `attr != "value"` (escaped), the exclude form of Eq.
func Neq(attr, value string) string {
	return attr + " != " + quote(value)
}

// InStrings builds `attr IN ["a", "b"]`, matching documents carrying any of the values
// (each quoted and escaped like Eq).
func InStrings(attr string, values []string) string {
	return attr + " IN " + quotedList(values)
}

// NotInStrings builds `attr NOT IN ["a", "b"]`, the exclude form of InStrings.
func NotInStrings(attr string, values []string) string {
	return attr + " NOT IN " + quotedList(values)
}

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// EqBool builds an equality fragment against a boolean attribute (unquoted).
func EqBool(attr string, v bool) string {
	return attr + " = " + strconv.FormatBool(v)
}

// IsEmpty builds `attr IS EMPTY`, matching documents whose array or string attribute is
// empty. It backs the regions "not specified" sentinel.
func IsEmpty(attr string) string {
	return attr + " IS EMPTY"
}

// IsNotEmpty builds `attr IS NOT EMPTY`, the exclude form of IsEmpty.
func IsNotEmpty(attr string) string {
	return attr + " IS NOT EMPTY"
}

// NotExists builds `attr NOT EXISTS`, matching documents without the attribute at all
// (how an omitempty field the posting never stated is stored).
func NotExists(attr string) string {
	return attr + " NOT EXISTS"
}

// Exists builds `attr EXISTS`, the complement of NotExists.
func Exists(attr string) string {
	return attr + " EXISTS"
}

// Gte builds a `attr >= n` numeric fragment.
func Gte(attr string, n int) string {
	return attr + " >= " + strconv.Itoa(n)
}

// Lte builds a `attr <= n` numeric fragment.
func Lte(attr string, n int) string {
	return attr + " <= " + strconv.Itoa(n)
}

// NotIn builds `attr NOT IN [a, b, c]` excluding a set of numeric ids. An empty set yields
// the empty string, so the caller adds no fragment (Meilisearch has no meaningful
// "NOT IN []").
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

// In builds `attr IN [a, b, c]` matching a set of numeric ids, the mirror of NotIn. An
// empty set yields the empty string, like NotIn.
func In(attr string, ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(attr)
	b.WriteString(" IN [")
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.FormatInt(id, 10))
	}
	b.WriteByte(']')
	return b.String()
}

// Filter nests OR-groups into one AND filter: fragments within a group are ORed, groups
// ANDed. Empty groups are dropped; nil means no filter.
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

// AndNotSkills narrows a base filter (nil or [][]string from FilterFromValues) to documents
// whose `skills` contain none of the given skills: one `skills != "<skill>"` AND group per
// skill, since array non-membership is per-value. Empty skills leave the base unchanged.
func AndNotSkills(base any, skills []string) any {
	groups, _ := base.([][]string)
	for _, s := range skills {
		if s != "" {
			groups = append(groups, []string{Neq("skills", s)})
		}
	}
	return Filter(groups...)
}

// AndInStrings narrows a base filter (nil or [][]string) to documents whose attr is one of
// values, as one more AND group holding a single IN. A caller whose set is EMPTY must answer
// empty itself: dropping the group would widen the search to the whole catalogue.
func AndInStrings(base any, attr string, values []string) any {
	groups, _ := base.([][]string)
	groups = append(groups, []string{InStrings(attr, values)})
	return Filter(groups...)
}

// AndSkillsPresent narrows a base filter to documents whose `skills` array is non-empty,
// so skill frequency is measured only against postings that list a tagged skill.
func AndSkillsPresent(base any) any {
	groups, _ := base.([][]string)
	groups = append(groups, []string{IsNotEmpty("skills")})
	return Filter(groups...)
}

// quote wraps a value in a Meilisearch string literal, escaping `"` and `\`.
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
