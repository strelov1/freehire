package salaryextract

import (
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
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
	reLabel  = regexp.MustCompile(`(?i)^(salary|pay|compensation|remuneration)\s*:\s*(.*)$`)
	reYear   = regexp.MustCompile(`(?i)(?:per\s+annum|per\s+year|/\s*year|/\s*yr\b|p\.a\.|annually|yearly)`)
	reMonth  = regexp.MustCompile(`(?i)(?:per\s+month|/\s*month|p/m\b|monthly)`)
	reHour   = regexp.MustCompile(`(?i)(?:per\s+hour|per\s+hr|/\s*hour|/\s*hr\b|p/h\b|hourly|an hour)`)
	reDay    = regexp.MustCompile(`(?i)(?:per\s+day|/\s*day\b|p/d\b|man-day|/\s*md\b)`)
	reUpTo   = regexp.MustCompile(`(?i)\bup to\b`)
	reFrom   = regexp.MustCompile(`(?i)\bfrom\b`)
	reMoney  = regexp.MustCompile(`(?i)(?:a\$|c\$|nz\$|us\$|s\$|hk\$|rp|£|€|\$|zł)\s*(\d[\d.,]*)\+?|(\d{1,3}(?:[.,]\d{3})+)\+?`)
	// reScaled is a "120k" or "1.5m" amount; reMoney would read it as 120 or 1.
	reScaled = regexp.MustCompile(`(?i)\d[km]\b`)
	// reOpenRange is a symboled amount whose dash-joined second bound carries no
	// symbol and no thousands grouping ("$50 - 60"), which reMoney does not read.
	reOpenRange = regexp.MustCompile(`(?i)(?:a\$|c\$|nz\$|us\$|s\$|hk\$|rp|£|€|\$|zł)\s*\d[\d.,]*\+?\s*[-–—]\s*\d`)
	reDotGrp    = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	reComGrp    = regexp.MustCompile(`^\d{1,3}(,\d{3})+(\.\d+)?$`)
	reEuDec     = regexp.MustCompile(`^\d{1,3}(\.\d{3})+,\d+$`)
	reDec       = regexp.MustCompile(`^\d+[.,]\d{1,2}$`)
	reInt       = regexp.MustCompile(`^\d+$`)
	reAUD       = regexp.MustCompile(`(?i)\baud\b`)
	reCAD       = regexp.MustCompile(`(?i)\bcad\b`)
	reNZD       = regexp.MustCompile(`(?i)\bnzd\b`)
	reHKD       = regexp.MustCompile(`(?i)\bhkd\b`)
	reSGD       = regexp.MustCompile(`(?i)\bsgd\b`)
	reUSD       = regexp.MustCompile(`(?i)\busd\b`)
	reIDR       = regexp.MustCompile(`(?i)\brp\b|\bidr\b`)
	reGBP       = regexp.MustCompile(`(?i)\bgbp\b`)
	reEUR       = regexp.MustCompile(`(?i)\beur\b`)
	rePLN       = regexp.MustCompile(`(?i)\bpln\b`)
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
	for _, line := range lines {
		m := reLabel.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		sal, ok := parseLine(m[2], countries)
		if !ok {
			continue
		}
		if got != nil && !same(*got, sal) {
			return Salary{}, false
		}
		got = &sal
		used = line
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
	if len(nums) == 1 && reOpenRange.MatchString(body) {
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
	case reFrom.MatchString(body) || strings.Contains(body, "+"):
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
	sal.Currency = currencyOf(body, countries)
	return sal, true
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

func currencyOf(line string, countries []string) string {
	switch {
	case strings.Contains(line, "US$") || reUSD.MatchString(line):
		return "USD"
	case strings.Contains(line, "CA$") || strings.Contains(line, "C$") || reCAD.MatchString(line):
		return "CAD"
	case strings.Contains(line, "NZ$") || reNZD.MatchString(line):
		return "NZD"
	case strings.Contains(line, "HK$") || reHKD.MatchString(line):
		return "HKD"
	case strings.Contains(line, "AU$") || strings.Contains(line, "A$") || reAUD.MatchString(line):
		return "AUD"
	case strings.Contains(line, "S$") || reSGD.MatchString(line):
		return "SGD"
	case reIDR.MatchString(line):
		return "IDR"
	case strings.Contains(line, "£") || reGBP.MatchString(line):
		return "GBP"
	case strings.Contains(line, "€") || reEUR.MatchString(line):
		return "EUR"
	case strings.Contains(line, "zł") || rePLN.MatchString(line):
		return "PLN"
	case strings.Contains(line, "$"):
		return bareDollar(countries)
	}
	return ""
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
