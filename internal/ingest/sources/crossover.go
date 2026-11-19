package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// crossover adapts Crossover (crossover.com), a recruiter of full-time remote and school-campus roles
// for Trilogy / ESW Capital, Alpha School and other brands. Boardless and multi-employer: the
// brand is read per posting. The /jobs page is an Angular SPA over two keyless services, so a
// crawl is two requests: the active pipelines (listing) and their CMS bodies, in one batch. See
// providers/crossover.md.
type crossover struct {
	http JSONGetter
}

const (
	crossoverListURL = "https://profile-api.crossover.com/pipelines?status=Active"
	crossoverBodyURL = "https://kontent-proxy.crossover.com/items?system.type=pipeline&depth=0" +
		"&elements=hook,what_you_will_be_doing,responsibilities,requirements,nice_to_have&system.codename[in]="
)

// NewCrossover builds the Crossover adapter over the given client.
func NewCrossover(c JSONGetter) Source { return crossover{http: c} }

func (crossover) Provider() string { return "crossover" }

func (crossover) boardless() {}

func (crossover) aggregator() {}

type crossoverPipeline struct {
	Code      string                 `json:"ProductCode"`
	Name      string                 `json:"Name"`
	Brand     *struct{ Name string } `json:"Brand__r"`
	Display   string                 `json:"DisplayUrl"`
	JobType   string                 `json:"Job_Type__c"`
	Model     string                 `json:"Work_Model__c"`
	Country   string                 `json:"Work_Country__c"`
	Yearly    float64                `json:"Yearly_Rate__c"`
	Locations *struct {
		Records []struct {
			Location *struct{ Name string } `json:"Location__r"`
		}
	} `json:"Work_Locations__r"`
}

// crossoverBodies is the CMS answer: each item's HTML sections, keyed by codename "pipeline_<code>".
type crossoverElement struct {
	Value any `json:"value"`
}

type crossoverBodies struct {
	Items []struct {
		System struct {
			Codename string `json:"codename"`
		} `json:"system"`
		Elements map[string]crossoverElement `json:"elements"`
	} `json:"items"`
}

func (s crossover) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var list struct {
		Records []crossoverPipeline `json:"records"`
	}
	if err := s.http.GetJSON(ctx, crossoverListURL, &list); err != nil {
		return nil, fmt.Errorf("crossover: %w", err)
	}
	if len(list.Records) == 0 {
		return nil, fmt.Errorf("crossover: empty pipeline list")
	}
	codenames := make([]string, 0, len(list.Records))
	for _, p := range list.Records {
		codenames = append(codenames, "pipeline_"+p.Code)
	}
	var bodies crossoverBodies
	if err := s.http.GetJSON(ctx, crossoverBodyURL+url.QueryEscape(strings.Join(codenames, ",")), &bodies); err != nil {
		return nil, fmt.Errorf("crossover: bodies: %w", err)
	}
	body := make(map[string]string, len(bodies.Items))
	for _, it := range bodies.Items {
		body[it.System.Codename] = crossoverBody(it.Elements)
	}
	jobs := make([]Job, 0, len(list.Records))
	for _, p := range list.Records {
		if p.Code == "" || p.Name == "" || p.Display == "" || p.Brand == nil {
			continue
		}
		desc := body["pipeline_"+p.Code]
		if desc == "" {
			jobs = append(jobs, unreadableDetail(p.Code, p.Display, p.Brand.Name))
			continue
		}
		jobs = append(jobs, p.toJob(desc))
	}
	return jobs, nil
}

// crossoverSections is the body order, each under its page heading.
var crossoverSections = []struct{ key, heading string }{
	{"hook", ""},
	{"what_you_will_be_doing", "What you will be doing"},
	{"responsibilities", "Responsibilities"},
	{"requirements", "Requirements"},
	{"nice_to_have", "Nice to have"},
}

func crossoverBody(el map[string]crossoverElement) string {
	var b strings.Builder
	for _, sec := range crossoverSections {
		v, _ := el[sec.key].Value.(string)
		if htmltext.ToText(v) == "" {
			continue
		}
		if sec.heading == "" {
			b.WriteString(v)
		} else {
			b.WriteString(headedSection(sec.heading, v))
		}
	}
	return sanitizeHTML(b.String())
}

func (p crossoverPipeline) toJob(desc string) Job {
	job := Job{
		ExternalID:  p.Code,
		URL:         p.Display,
		Title:       p.Name,
		Company:     p.Brand.Name,
		Location:    p.location(),
		Description: desc,
	}
	switch p.Model {
	case "Remote":
		job.WorkMode = "remote"
	case "Hybrid":
		job.WorkMode = "hybrid"
	case "On-site":
		job.WorkMode = "onsite"
	}
	job.Remote = job.WorkMode == "remote"
	switch p.JobType {
	case "full-time":
		job.EmploymentType = "full_time"
	case "part-time":
		job.EmploymentType = "part_time"
	}
	if p.Yearly > 0 {
		y := int(p.Yearly)
		job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = &y, &y, "USD", "year"
	}
	return job
}

// location lists the campus cities of an on-site posting, else the country, else "Remote".
func (p crossoverPipeline) location() string {
	var names []string
	if p.Locations != nil {
		for _, r := range p.Locations.Records {
			if r.Location != nil && r.Location.Name != "" {
				names = append(names, r.Location.Name)
			}
		}
	}
	switch {
	case len(names) > 0:
		return strings.Join(names, "; ")
	case p.Country != "":
		return p.Country
	}
	return "Remote"
}
