package classify

import (
	"strings"
	"unicode"
)

func FoldTerm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	s = strings.ReplaceAll(s, "&", " and ")
	var b strings.Builder
	hyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if !hyphen && b.Len() > 0 {
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

const (
	GapInsert    = "insert"
	GapIncrement = "increment"

	GapRefuseEmpty = "empty"
	GapRefuseLong  = "long"
	GapRefuseCap   = "cap"

	gapDailyCap = 20
)

type GapRow struct {
	Status      string
	Raw         string
	Count       int
	SubmittedBy int64
}

type GapPlan struct {
	Refuse     string
	Normalized string
	Op         string
	Status     string
}

func PlanGap(raw string, writesToday int, existing []GapRow) GapPlan {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > 80 {
		return GapPlan{Refuse: GapRefuseLong}
	}
	normalized := FoldTerm(trimmed)
	if normalized == "" {
		return GapPlan{Refuse: GapRefuseEmpty}
	}
	if writesToday >= gapDailyCap {
		return GapPlan{Refuse: GapRefuseCap}
	}
	for _, row := range existing {
		if row.Status == "pending" {
			return GapPlan{Normalized: normalized, Op: GapIncrement, Status: "pending"}
		}
	}
	for _, row := range existing {
		if row.Status == "accepted" {
			return GapPlan{Normalized: normalized, Op: GapIncrement, Status: "accepted"}
		}
	}
	return GapPlan{Normalized: normalized, Op: GapInsert, Status: "pending"}
}
