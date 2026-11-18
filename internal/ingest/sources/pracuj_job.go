package sources

import (
	"html"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// pracujJob maps an offer page and its search result to a Job. ok is false for one with no
// title, employer or body.
func pracujJob(o pracujOffer, p pracujPosting) (Job, bool) {
	a := o.Attributes
	j := pracujBaseJob(p)
	if t := collapseSpace(a.Title); t != "" {
		j.Title = t
	}
	if co := collapseSpace(a.Employer); co != "" {
		j.Company = co
	}
	body := pracujBody(o.Sections)
	if pay := pracujPayParagraph(a.Employment.Contracts); pay != "" {
		body = pay + body
	}
	j.Description = sanitizeHTML(body)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	j.Location, j.Countries = pracujPlace(o)
	if j.Location == "" {
		j.Location = collapseSpace(p.Offers[0].Workplace)
	}
	if t := parseRFC3339(strings.TrimSpace(o.Publication.Initial)); t != nil {
		j.PostedAt = NotFuture(t)
	}
	codes := make([]string, 0, len(a.Employment.WorkModes))
	for _, m := range a.Employment.WorkModes {
		codes = append(codes, m.Code)
	}
	if a.Employment.EntirelyRemote {
		codes = append(codes, "home-office")
	}
	if m := pracujWorkMode(codes); m != "" {
		j.WorkMode, j.Remote = m, m == "remote"
	}
	if pracujTechCategories[a.Leading.Parent.ID] {
		j.IsTechHint = true
	}
	levels := make([]int, 0, len(a.Employment.PositionLevels))
	for _, l := range a.Employment.PositionLevels {
		levels = append(levels, l.ID)
	}
	j.Seniority = pracujSeniority(levels)
	j.EmploymentType = pracujEmploymentType(a.Employment.WorkSchedules, a.Employment.Contracts)
	j.Skills = pracujSkills(o.Sections)
	if lo, hi, period, ok := pracujSalary(a.Employment.Contracts); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "PLN", period
	}
	return j, true
}

// pracujPlace reads the posting's first workplace as its location, and every workplace's country:
// one in Poland is "pl", one abroad is the country its page names (in the posting's language, so
// a Polish page's "Niemcy" resolves to nothing and is left to the dictionaries).
func pracujPlace(o pracujOffer) (string, []string) {
	var loc string
	var countries []string
	for _, w := range o.Attributes.Workplaces {
		code := "pl"
		if w.IsAbroad {
			code = location.NormalizeCountry(strings.TrimSpace(w.Country.Name))
		}
		if code != "" && !slices.Contains(countries, code) {
			countries = append(countries, code)
		}
		if loc != "" {
			continue
		}
		if w.Inland != nil && strings.TrimSpace(w.Inland.Location.Name) != "" {
			loc = collapseSpace(w.Inland.Location.Name)
		} else {
			loc = collapseSpace(w.Display)
		}
	}
	return loc, countries
}

// pracujWorkMode reads the posting's stated arrangements, the most remote one winning, from the
// search result's labels ("Praca zdalna", "Praca hybrydowa (do uzgodnienia)") or the offer page's
// codes ("home-office", "hybrid", "full-office"). "Praca mobilna" (field work) says nothing.
func pracujWorkMode(modes []string) string {
	set := map[string]bool{}
	for _, m := range modes {
		m = strings.ToLower(collapseSpace(m))
		switch {
		case m == "home-office" || strings.HasPrefix(m, "praca zdalna"):
			set["remote"] = true
		case m == "hybrid" || strings.HasPrefix(m, "praca hybrydowa"):
			set["hybrid"] = true
		case m == "full-office" || strings.HasPrefix(m, "praca stacjonarna"):
			set["onsite"] = true
		}
	}
	for _, m := range []string{"remote", "hybrid", "onsite"} {
		if set[m] {
			return m
		}
	}
	return ""
}

// pracujSeniorityByLevel maps the site's position-level ids. Assistant, coordinator, manager,
// director and manual worker name a role, not a seniority, and map to nothing.
var pracujSeniorityByLevel = map[int]string{
	1:  "intern", // praktykant / stażysta
	17: "junior", // młodszy specjalista (junior)
	4:  "middle", // specjalista (mid / regular)
	18: "senior", // starszy specjalista (senior)
	19: "senior", // ekspert
	21: "c_level",
}

// pracujSeniority is the seniority every stated level agrees on, or "" when they disagree ("mid /
// senior" is two levels) or none maps.
func pracujSeniority(levels []int) string {
	out := ""
	for _, l := range levels {
		s := pracujSeniorityByLevel[l]
		if s == "" || (out != "" && s != out) {
			return ""
		}
		out = s
	}
	return out
}

// pracujEmploymentType maps a single stated schedule (full-time 0, part-time 1); an internship
// contract (7) as the only contract wins over the schedule.
func pracujEmploymentType(schedules []struct {
	ID int `json:"id"`
}, contracts []pracujContract) string {
	if len(contracts) == 1 && contracts[0].ID == 7 {
		return "internship"
	}
	if len(schedules) != 1 {
		return ""
	}
	switch schedules[0].ID {
	case 0:
		return "full_time"
	case 1:
		return "part_time"
	}
	return ""
}

// pracujContractUoP is "umowa o pracę", the contract of employment; pracujContractB2B is the B2B
// contract, paid net of VAT.
const (
	pracujContractUoP = 0
	pracujContractB2B = 3
)

// pracujBody renders the posting's sections as HTML: a heading per titled section, bullets as a
// list, paragraphs as paragraphs, and a dictionary (technologies, benefits) as a list of its
// item names. A gallery is skipped.
func pracujBody(sections []pracujSection) string {
	var b strings.Builder
	var render func(secs []pracujSection, depth int)
	render = func(secs []pracujSection, depth int) {
		for _, sec := range secs {
			if t := collapseSpace(sec.Title); t != "" {
				tag := "h2"
				if depth > 0 {
					tag = "h3"
				}
				b.WriteString("<" + tag + ">" + html.EscapeString(t) + "</" + tag + ">\n")
			}
			if m := sec.Model; m != nil {
				var items []string
				switch {
				case len(m.Bullets) > 0:
					items = m.Bullets
				case len(m.Items) > 0 || len(m.CustomItems) > 0:
					for _, it := range m.Items {
						items = append(items, it.Name)
					}
					for _, it := range m.CustomItems {
						items = append(items, it.Name)
					}
				}
				if len(items) > 0 {
					b.WriteString("<ul>")
					for _, it := range items {
						if it = strings.TrimSpace(it); it != "" {
							b.WriteString("<li>" + html.EscapeString(it) + "</li>")
						}
					}
					b.WriteString("</ul>\n")
				}
				for _, para := range m.Paragraphs {
					if para = strings.TrimSpace(para); para != "" {
						b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(para), "\n", "<br>") + "</p>\n")
					}
				}
			}
			render(sec.Subs, depth+1)
		}
	}
	render(sections, 0)
	return b.String()
}

// pracujSkills canonicalizes the technologies the poster picked (expected and optional) through
// the skill dictionary. Canonicalize, not Parse: these are discrete names from a structured
// field, not prose to mine.
func pracujSkills(sections []pracujSection) []string {
	var names []string
	var collect func(secs []pracujSection)
	collect = func(secs []pracujSection) {
		for _, sec := range secs {
			if strings.HasPrefix(sec.Type, "technologies") && sec.Model != nil {
				for _, it := range sec.Model.Items {
					names = append(names, it.Name)
				}
				for _, it := range sec.Model.CustomItems {
					names = append(names, it.Name)
				}
			}
			collect(sec.Subs)
		}
	}
	collect(sections)
	if len(names) == 0 {
		return nil
	}
	return skilltag.Canonicalize(names)
}
