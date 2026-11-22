package sources

import (
	"regexp"
	"strconv"
	"strings"
)

// jobsczDetail is what a posting's own page (/rpd/<id>/) carries: the header, the "Informace o
// pozici" block and the body. The page has no JSON-LD and no embedded state; it is plain
// server-rendered markup.
type jobsczDetail struct {
	Title   string
	Company string
	// Address is the street address line the page links to its map.
	Address string
	// Salary is the pay line ("50 000 – 85 000 Kč"), empty when the posting states none.
	Salary string
	// Info maps a label of the "Informace o pozici" block to the paragraphs under it:
	// "Zařazeno" (the fields, then the professions), "Typ pracovního poměru", "Požadované jazyky".
	Info map[string][]string
	// BodyHTML is the posting's rich text.
	BodyHTML string
}

var (
	jobsczH1Re       = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	jobsczAddressRe  = regexp.MustCompile(`(?s)data-test="jd-info-location"[^>]*>(.*?)</a>`)
	jobsczSalaryRe   = regexp.MustCompile(`(?s)data-test="jd-salary".*?<p[^>]*>(.*?)</p>`)
	jobsczInfoRe     = regexp.MustCompile(`(?s)class="IconWithText"\s*>\s*(?:<svg.*?</svg>\s*)?<span\s+class="accessibility-hidden"\s*>\s*(.*?)\s*</span>((?:\s*<p[^>]*>.*?</p>)+)`)
	jobsczParagraphs = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
)

// jobsczBodyMarker opens the rich-text body; only a posting page the site renders itself has it.
const jobsczBodyMarker = `data-jobad="body"`

// jobsczParseDetail reads a posting page. ok is false for a page without the body container:
// the shell of a partner career site, or a page whose markup changed.
func jobsczParseDetail(page string) (jobsczDetail, bool) {
	body, ok := jobsczBody(page)
	if !ok {
		return jobsczDetail{}, false
	}
	d := jobsczDetail{BodyHTML: body, Info: map[string][]string{}}
	if m := jobsczH1Re.FindStringSubmatch(page); m != nil {
		d.Title = jobsczText(m[1])
	}
	if m := jobsczAddressRe.FindStringSubmatch(page); m != nil {
		d.Address = jobsczText(m[1])
	}
	if m := jobsczSalaryRe.FindStringSubmatch(page); m != nil {
		d.Salary = jobsczText(m[1])
	}
	for _, m := range jobsczInfoRe.FindAllStringSubmatch(page, -1) {
		label := jobsczText(m[1])
		if _, dup := d.Info[label]; dup {
			// The block is rendered twice, above and below the body.
			continue
		}
		for _, p := range jobsczParagraphs.FindAllStringSubmatch(m[2], -1) {
			d.Info[label] = append(d.Info[label], jobsczText(p[1]))
		}
	}
	if c := d.Info["Společnost"]; len(c) > 0 {
		d.Company = c[0]
	}
	return d, true
}

// jobsczBody cuts the body container out of the page: from the end of its opening tag to the
// div that closes it. The body holds divs of its own, so the end is found by counting.
func jobsczBody(page string) (string, bool) {
	at := strings.Index(page, jobsczBodyMarker)
	if at < 0 {
		return "", false
	}
	open := strings.Index(page[at:], ">")
	if open < 0 {
		return "", false
	}
	start := at + open + 1
	depth := 1
	for i := start; i < len(page); {
		next := strings.Index(page[i:], "<")
		if next < 0 {
			break
		}
		i += next
		switch {
		case strings.HasPrefix(page[i:], "</div>"):
			depth--
			if depth == 0 {
				return page[start:i], true
			}
			i += len("</div>")
		case strings.HasPrefix(page[i:], "<div") && i+4 < len(page) && (page[i+4] == ' ' || page[i+4] == '>' || page[i+4] == '\n'):
			depth++
			i += len("<div")
		default:
			i++
		}
	}
	return "", false
}

func (d jobsczDetail) toJob(it jobsczItem) (Job, bool) {
	title := d.Title
	if title == "" {
		title = it.Title
	}
	company := d.Company
	if company == "" {
		company = it.Company
	}
	description := sanitizeHTML(d.BodyHTML)
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	location := it.Location
	if location == "" {
		location = d.Address
	}
	workMode := jobsczWorkMode(it.Tags)
	job := Job{
		ExternalID:     it.ID,
		URL:            jobsczPostingURL(it.ID),
		Title:          title,
		Company:        company,
		Location:       location,
		Description:    description,
		WorkMode:       workMode,
		Remote:         workMode == "remote",
		EmploymentType: jobsczEmploymentType(d.Info["Typ pracovního poměru"]),
		// A Czech place named bare ("Brno – Medlánky") states no country, and the board is the
		// Czech one; the country is read only when the line states none.
		FallbackCountries: []string{"cz"},
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = jobsczSalary(d.Salary)
	// The site files a posting under fields of its own ("Zařazeno", fields first); the three
	// "IS/IT: ..." fields state that the employer posted it as technical.
	if fields := d.Info["Zařazeno"]; len(fields) > 0 {
		// The paragraph reads "<fields> , <professions>"; the space before the comma is what tells
		// the two lists apart from the commas inside each.
		// Team stays unset: the field names hold commas of their own, so one cannot be cut out.
		fieldList, _, _ := strings.Cut(fields[0], " , ")
		job.IsTechHint = jobsczTechField(fieldList)
	}
	return job, true
}

// jobsczTechField is true when a field list names one of the IT fields.
func jobsczTechField(fields string) bool {
	return strings.Contains(fields, "IS/IT:")
}

// jobsczWorkMode reads the card's work-arrangement tags: "Práce převážně z domova" is remote,
// "Možnost práce z domova" and "Možnost občasné práce z domova" are a home option beside the
// office, which is hybrid.
func jobsczWorkMode(tags []string) string {
	mode := ""
	for _, t := range tags {
		t = strings.ToLower(t)
		switch {
		case strings.Contains(t, "převážně z domova"):
			return "remote"
		case strings.Contains(t, "z domova"):
			mode = "hybrid"
		}
	}
	return mode
}

// jobsczEmploymentType maps the "Typ pracovního poměru" paragraph. A posting that states both a
// full and a part-time form is left to the dictionaries, since either is true of it.
func jobsczEmploymentType(values []string) string {
	t := strings.ToLower(strings.Join(values, ", "))
	full := strings.Contains(t, "plný úvazek")
	part := strings.Contains(t, "zkrácený úvazek")
	switch {
	case full && part:
		return ""
	case full:
		return "full_time"
	case part:
		return "part_time"
	case strings.Contains(t, "stáž") || strings.Contains(t, "trainee"):
		return "internship"
	case strings.Contains(t, "dohod") || strings.Contains(t, "živnost"):
		return "contract"
	}
	return ""
}

var jobsczNumberRe = regexp.MustCompile(`\d+(?: \d{3})*`)

// jobsczSalary reads the page's pay line, "50 000 – 85 000 Kč" or "34 200 Kč". The site states
// gross pay per month in crowns; a line naming an hour or another currency is read as such, and
// one in a currency this does not know reads as no salary.
func jobsczSalary(line string) (lo, hi *int, currency, period string) {
	line = strings.NewReplacer("\u200d", "", " ", " ", " ", " ").Replace(line)
	line = strings.Join(strings.Fields(line), " ")
	nums := jobsczNumberRe.FindAllString(line, -1)
	if len(nums) == 0 || len(nums) > 2 {
		return nil, nil, "", ""
	}
	switch low := strings.ToLower(line); {
	case strings.Contains(low, "kč"), strings.Contains(low, "czk"):
		currency = "CZK"
	case strings.Contains(low, "€"), strings.Contains(low, "eur"):
		currency = "EUR"
	default:
		return nil, nil, "", ""
	}
	period = "month"
	if strings.Contains(strings.ToLower(line), "hod") {
		period = "hour"
	}
	parse := func(s string) *int {
		n, err := strconv.Atoi(strings.ReplaceAll(s, " ", ""))
		if err != nil || n <= 0 {
			return nil
		}
		return &n
	}
	lo = parse(nums[0])
	hi = lo
	if len(nums) == 2 {
		hi = parse(nums[1])
	}
	if lo == nil || hi == nil {
		return nil, nil, "", ""
	}
	return lo, hi, currency, period
}
