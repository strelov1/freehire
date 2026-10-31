package salaryextract

import (
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Salary struct {
	Min      *int
	Max      *int
	Currency string
	Period   string
}

var (
	blockTag = regexp.MustCompile(`(?i)<\s*/?\s*(?:p|div|br|li|h[1-6]|tr)\b[^>]*>`)
	anyTag   = regexp.MustCompile(`<[^>]+>`)
	reLabel  = regexp.MustCompile(`(?i)^(salary(?: range)?|pay|compensation|remuneration)\s*:\s*(.*)$`)
	reYear   = regexp.MustCompile(`(?i)(?:per\s+annum|per\s+year|/\s*year|/\s*yr\b|p\.a\.|annually|yearly)`)
	reMonth  = regexp.MustCompile(`(?i)(?:per\s+month|/\s*month|p/m\b|monthly)`)
	reHour   = regexp.MustCompile(`(?i)(?:per\s+hour|per\s+hr|/\s*hour|/\s*hr\b|p/h\b|hourly|an hour)`)
	reDay    = regexp.MustCompile(`(?i)(?:per\s+day|/\s*day\b|p/d\b|man-day|/\s*md\b)`)
	reUpTo   = regexp.MustCompile(`(?i)\bup to\b`)
	reFrom   = regexp.MustCompile(`(?i)\bfrom\b`)
	reMoney  = regexp.MustCompile(`(?i)(?:ca\$|au\$|a\$|c\$|nz\$|us\$|s\$|hk\$|rp|£|€|\$|zł)\s*(\d[\d.,]*)\+?|(\d{1,3}(?:[.,]\d{3})+(?:[.,]\d{1,2})?)\+?`)
	// reScaled is a money token with a k/m suffix ("$120k", "1,200k", "100,000 - 150k").
	// reMoney would keep the digits and drop the suffix. A benefit word ("401k")
	// is not a money token and is not a range bound, so it is not this.
	reScaled = regexp.MustCompile(`(?i)(?:` +
		`(?:(?:ca\$|au\$|a\$|c\$|nz\$|us\$|s\$|hk\$|rp|£|€|\$|zł)\s*\d[\d.,]*|\d{1,3}(?:[.,]\d{3})+(?:[.,]\d{1,2})?|[-–—]\s*\d[\d.,]*)\s*[km]\b` +
		`|\d[\d.,]*\s*[km]\b\s*[-–—])`)
	// reOpenRange is a dash-joined second bound reMoney does not read ("$50 - 60").
	reOpenRange     = regexp.MustCompile(`(?i)(?:ca\$|au\$|a\$|c\$|nz\$|us\$|s\$|hk\$|rp|£|€|\$|zł)\s*\d[\d.,]*\+?\s*[-–—]\s*\d`)
	reTrailRange    = regexp.MustCompile(`(?i)\d[\d.,]+\s*(?:us\$|ca\$|au\$|nz\$|hk\$|a\$|c\$|s\$|rp|zł|£|€|\$|usd|aud|cad|nzd|hkd|sgd|gbp|eur|pln|idr)\s*[-–—]\s*\d`)
	reBenefitClause = regexp.MustCompile(`(?i)` +
		`(?:(?:[$£€]|zł|(?:us|ca|au|nz|hk|a|c|s)\$|rp)\s*)?` +
		`\d[\d.,]*\s*` +
		`(?:(?:[$£€]|zł|(?:us|ca|au|nz|hk|a|c|s)\$|rp)\s*)?` +
		`(?:sign[\s-]?on\s+|signing\s+)?` +
		`(?:bonus(?:es)?|benefits?)\b` +
		`|(?:sign[\s-]?on|signing|bonus(?:es)?|benefits?)\b`)
	reDotGrp   = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	reComGrp   = regexp.MustCompile(`^\d{1,3}(,\d{3})+(\.\d+)?$`)
	reEuDec    = regexp.MustCompile(`^\d{1,3}(\.\d{3})+,\d+$`)
	reDec      = regexp.MustCompile(`^\d+[.,]\d{1,2}$`)
	reInt      = regexp.MustCompile(`^\d+$`)
	reLeadISO  = regexp.MustCompile(`(?i)^(usd|aud|cad|nzd|hkd|sgd|gbp|eur|pln|idr)\b`)
	reTrailISO = regexp.MustCompile(`(?i)\b(usd|aud|cad|nzd|hkd|sgd|gbp|eur|pln|idr)$`)
)

var dollarCurrency = map[string]string{
	"us": "USD",
	"au": "AUD",
	"ca": "CAD",
	"nz": "NZD",
	"sg": "SGD",
	"hk": "HKD",
}

var periodSpecs = []struct {
	re   *regexp.Regexp
	name string
}{
	{reHour, "hour"},
	{reDay, "day"},
	{reMonth, "month"},
	{reYear, "year"},
}

func Parse(description string, countries []string) (Salary, bool) {
	lines := plain(description)
	var got *Salary
	var used string
	for i, line := range lines {
		m := reLabel.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		body := m[2]
		if body == "" {
			// Workday renders the label and its value in separate HTML blocks.
			// Only the next non-empty line belongs to that label.
			for _, next := range lines[i+1:] {
				if next == "" {
					continue
				}
				if loc := reMoney.FindStringIndex(next); loc != nil && loc[0] == 0 && !benefitOnly(next) {
					body = withoutBenefit(next)
				}
				break
			}
		}
		sal, ok := parseLine(body, countries)
		if !ok {
			continue
		}
		if got != nil && !same(*got, sal) {
			return Salary{}, false
		}
		got = &sal
		used = body
	}
	if got == nil {
		return Salary{}, false
	}
	if contradicts(lines, used, *got) {
		return Salary{}, false
	}
	return *got, true
}

func parseLine(body string, countries []string) (Salary, bool) {
	body = strings.TrimSpace(body)
	nums := amountsIn(body)
	if len(nums) == 0 || len(nums) > 2 || reScaled.MatchString(body) {
		return Salary{}, false
	}
	if len(nums) == 1 && (reOpenRange.MatchString(body) || reTrailRange.MatchString(body)) {
		return Salary{}, false
	}
	var sal Salary
	switch {
	case len(nums) == 2:
		lo, hi := nums[0], nums[1]
		if lo > hi {
			lo, hi = hi, lo
		}
		sal.Min, sal.Max = ip(lo), ip(hi)
	case reUpTo.MatchString(body):
		sal.Max = ip(nums[0])
	case reFrom.MatchString(body) || figurePlus(body):
		sal.Min = ip(nums[0])
	default:
		n := nums[0]
		sal.Min, sal.Max = ip(n), ip(n)
	}
	period, ok := periodOf(body)
	if !ok {
		return Salary{}, false
	}
	sal.Period = period
	larger := nums[0]
	if len(nums) == 2 && nums[1] > larger {
		larger = nums[1]
	}
	if sal.Period == "" && larger < 1000 {
		return Salary{}, false
	}
	cur, okCur := currencyOf(body, countries)
	if !okCur {
		return Salary{}, false
	}
	sal.Currency = cur
	return sal, true
}

func benefitOnly(line string) bool {
	if !reBenefitClause.MatchString(line) {
		return false
	}
	return len(amountsIn(withoutBenefit(line))) == 0
}

func withoutBenefit(line string) string {
	return strings.TrimSpace(reBenefitClause.ReplaceAllString(line, " "))
}

func figurePlus(body string) bool {
	for _, loc := range reMoney.FindAllStringIndex(body, -1) {
		if strings.HasSuffix(body[loc[0]:loc[1]], "+") {
			return true
		}
	}
	return false
}

func contradicts(lines []string, used string, sal Salary) bool {
	if sal.Period == "" {
		return false
	}
	rest := strings.Replace(strings.Join(lines, "\n"), used, "\n", 1)
	for _, spec := range periodSpecs {
		if spec.name == sal.Period {
			continue
		}
		for _, loc := range spec.re.FindAllStringIndex(rest, -1) {
			start := loc[0] - 120
			if start < 0 {
				start = 0
			}
			end := loc[1] + 120
			if end > len(rest) {
				end = len(rest)
			}
			for _, n := range amountsIn(rest[start:end]) {
				if !isBound(n, sal) {
					return true
				}
			}
		}
	}
	return false
}

func plain(s string) []string {
	s = html.UnescapeString(s)
	if strings.Contains(s, "<") {
		s = blockTag.ReplaceAllString(s, "\n")
		s = anyTag.ReplaceAllString(s, " ")
	}
	s = strings.ReplaceAll(s, "\u00a0", " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return lines
}

func amountsIn(s string) []int {
	var out []int
	for _, m := range reMoney.FindAllStringSubmatch(s, -1) {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		n, ok := parseNumber(raw)
		if ok {
			out = append(out, n)
		}
	}
	return out
}

func parseNumber(s string) (int, bool) {
	s = strings.Trim(s, ".,")
	switch {
	case reEuDec.MatchString(s):
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
		return roundFloat(s)
	case reDotGrp.MatchString(s):
		s = strings.ReplaceAll(s, ".", "")
	case reComGrp.MatchString(s):
		s = strings.ReplaceAll(s, ",", "")
		if strings.Contains(s, ".") {
			return roundFloat(s)
		}
	case reDec.MatchString(s):
		s = strings.Replace(s, ",", ".", 1)
		return roundFloat(s)
	case reInt.MatchString(s):
	default:
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func roundFloat(s string) (int, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return int(math.Round(f)), true
}

func periodOf(s string) (string, bool) {
	found := ""
	for _, spec := range periodSpecs {
		if spec.re.MatchString(s) {
			if found != "" && found != spec.name {
				return "", false
			}
			found = spec.name
		}
	}
	return found, true
}

func currencyOf(line string, countries []string) (string, bool) {
	found := ""
	set := false
	for _, loc := range reMoney.FindAllStringSubmatchIndex(line, -1) {
		cur, ok := currencyAt(line, loc[0], loc[1], countries)
		if !ok {
			return "", false
		}
		if cur == "" {
			continue
		}
		if set && cur != found {
			return "", false
		}
		found = cur
		set = true
	}
	return found, true
}

var currencyMarks = []string{"US$", "CA$", "AU$", "NZ$", "HK$", "A$", "C$", "S$", "Rp", "zł", "£", "€", "$"}

func currencyAt(line string, start, end int, countries []string) (string, bool) {
	marker := strings.TrimSpace(extendMarker(line, start, line[start:end]))
	near := neighborMark(line, start, end)
	explicit := explicitCode(marker)
	beside := explicitCode(near)
	if explicit != "" && beside != "" && explicit != beside {
		return "", false
	}
	if explicit != "" {
		return explicit, true
	}
	if beside != "" {
		return beside, true
	}
	if marker == "$" {
		return bareDollar(countries), true
	}
	return "", true
}

func extendMarker(line string, start int, matched string) string {
	i := 0
	for i < len(matched) && (matched[i] < '0' || matched[i] > '9') {
		i++
	}
	j := start
	n := 0
	for j > 0 && n < 2 {
		c := line[j-1]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			break
		}
		j--
		n++
	}
	return line[j:start] + matched[:i]
}

func neighborMark(line string, start, end int) string {
	if m := leadingMark(strings.TrimLeftFunc(line[end:], unicode.IsSpace)); m != "" {
		return m
	}
	return trailingMark(strings.TrimRightFunc(line[:start], unicode.IsSpace))
}

func leadingMark(s string) string {
	if s == "" {
		return ""
	}
	if m := reLeadISO.FindString(s); m != "" {
		return m
	}
	for _, sym := range currencyMarks {
		if strings.HasPrefix(strings.ToUpper(s), strings.ToUpper(sym)) {
			return sym
		}
	}
	return ""
}

func trailingMark(s string) string {
	if s == "" {
		return ""
	}
	if m := reTrailISO.FindString(s); m != "" {
		return m
	}
	upper := strings.ToUpper(s)
	for _, sym := range currencyMarks {
		if strings.HasSuffix(upper, strings.ToUpper(sym)) {
			return sym
		}
	}
	return ""
}

func explicitCode(marker string) string {
	switch strings.ToUpper(strings.TrimSpace(marker)) {
	case "US$", "USD":
		return "USD"
	case "CA$", "C$", "CAD":
		return "CAD"
	case "AU$", "A$", "AUD":
		return "AUD"
	case "NZ$", "NZD":
		return "NZD"
	case "HK$", "HKD":
		return "HKD"
	case "S$", "SGD":
		return "SGD"
	case "£", "GBP":
		return "GBP"
	case "€", "EUR":
		return "EUR"
	case "ZŁ", "PLN":
		return "PLN"
	case "RP", "IDR":
		return "IDR"
	default:
		return ""
	}
}

func bareDollar(countries []string) string {
	cur := ""
	for _, c := range countries {
		next, ok := dollarCurrency[strings.ToLower(c)]
		if !ok {
			continue
		}
		if cur == "" {
			cur = next
			continue
		}
		if cur != next {
			return ""
		}
	}
	return cur
}

func isBound(n int, s Salary) bool {
	return (s.Min != nil && *s.Min == n) || (s.Max != nil && *s.Max == n)
}

func same(a, b Salary) bool {
	return eq(a.Min, b.Min) && eq(a.Max, b.Max) && a.Currency == b.Currency && a.Period == b.Period
}

func eq(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func ip(n int) *int { return &n }
