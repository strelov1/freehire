package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// isolvedFamily adapts the iSolved Hire and ApplicantPro career sites — one Vue-based platform
// served under two host domains. The board is the tenant subdomain, forming host
// "<board>.<host>". A company's postings are enumerated from its sitemap.xml (the /jobs/<id>
// URLs); each posting's fields come from that detail page's schema.org JobPosting ld+json —
// the sitemap-plus-ld+json-detail shape shared with successfactors/clinch.
type isolvedFamily struct {
	http     isolvedHTTP
	provider string
	host     string
}

// isolvedHTTP is the client capability the family needs: the sitemap as XML and each detail
// page as parsed HTML (for its ld+json).
type isolvedHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewIsolvedHire builds the *.isolvedhire.com adapter.
func NewIsolvedHire(c isolvedHTTP) Source {
	return isolvedFamily{http: c, provider: "isolvedhire", host: "isolvedhire.com"}
}

// NewApplicantPro builds the *.applicantpro.com adapter (same platform, different host).
func NewApplicantPro(c isolvedHTTP) Source {
	return isolvedFamily{http: c, provider: "applicantpro", host: "applicantpro.com"}
}

func (s isolvedFamily) Provider() string { return s.provider }

// isolvedJobID captures the numeric posting id from a /jobs/<id> URL. The sitemap lists both
// /jobs/<id> and /jobs/<id>.html plus marketing/classification pages, so the id (deduped) is
// the stable enumeration key.
var isolvedJobID = regexp.MustCompile(`/jobs/(\d+)`)

func (s isolvedFamily) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	smURL := fmt.Sprintf("https://%s.%s/sitemap.xml", e.Board, s.host)
	if err := s.http.GetXML(ctx, smURL, &sitemap); err != nil {
		return nil, fmt.Errorf("%s: sitemap %q: %w", s.provider, e.Board, err)
	}

	seen := map[string]struct{}{}
	var ids []string
	for _, u := range sitemap.URLs {
		if m := isolvedJobID.FindStringSubmatch(u.Loc); m != nil {
			if _, ok := seen[m[1]]; !ok {
				seen[m[1]] = struct{}{}
				ids = append(ids, m[1])
			}
		}
	}

	return fetchDetails(ids, defaultDetailWorkers, func(id string) (Job, bool) {
		return s.detail(ctx, e, id)
	}), nil
}

// detail fetches one posting's detail page and maps its JobPosting ld+json to a Job, returning
// ok=false when the fetch fails or the page carries no JobPosting.
func (s isolvedFamily) detail(ctx context.Context, e CompanyEntry, id string) (Job, bool) {
	loc := fmt.Sprintf("https://%s.%s/jobs/%s", e.Board, s.host, id)
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p isolvedPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := joinNonEmpty(
		p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressRegion,
		p.JobLocation.Address.AddressCountry,
	)

	// datePosted is a space-separated "2006-01-02 15:04:05" with no zone; the date part is
	// the reliable signal, so posted_at is date-granularity.
	posted := p.DatePosted
	if len(posted) > 10 {
		posted = posted[:10]
	}

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       p.Title,
		Company:     firstNonEmpty(e.Company, p.HiringOrganization.Name),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseDate(posted),
	}, true
}

// isolvedPosting is the schema.org JobPosting decoded from a detail page's ld+json.
type isolvedPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}
