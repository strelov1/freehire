package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// hrManager adapts HR-Manager (hr-manager.net), a Danish ATS whose customers' postings are
// served from candidate.hr-manager.net. A board is the customer ALIAS ("energinet",
// "rema1000") — the key the platform's own public job-portal API is addressed by. A posting URL
// carries only the customer's numeric cid, which the API does not accept; the alias is what
// the platform's own embed snippets name (api.hr-manager.net/JobPortal.svc/<alias>/...).
//
// The listing is GET api.hr-manager.net/jobportal.svc/<alias>/positionlist/json/, keyless and
// reachable from a datacenter IP. With incads=1 each item carries its advertisement HTML, so a
// row is the whole posting and there is no detail request. Without take the API answers the
// first 25 rows only; PositionCountSearch is the customer's total.
//
// fullBoardListing: the board is ONE request asking for more rows than the customer holds, so no
// page boundary can cut it; a customer over hrManagerTake fails rather than paging. The rows
// must be distinct and agree with the response's own PositionCountList, and may fall short of
// PositionCountSearch by at most hrManagerMaxHidden (see there).
type hrManager struct {
	http JSONGetter
}

const (
	hrManagerListURL = "https://api.hr-manager.net/jobportal.svc/%s/positionlist/json/"
	// hrManagerTake is the most rows the one listing request asks for: three times the largest
	// customer seen, and a ceiling that fails the crawl when a customer outgrows it.
	hrManagerTake = 2000
	// hrManagerMaxHidden is how many postings PositionCountSearch may count that no public view
	// serves: a customer can count a posting that its API (at any take), RSS variant and HTML
	// listing all omit, because the server skips the hidden row's position. A shortfall beyond
	// this is not a hidden posting but a listing we did not fully read.
	hrManagerMaxHidden = 2
	// hrManagerOpenApplication is the project type of a customer's standing "unsolicited
	// application" entry, which the list carries beside its real postings.
	hrManagerOpenApplication = "OpenApplication"
)

// hrManagerAliasRE is the shape of a customer alias seen in the wild: letters, digits,
// underscores and hyphens ("bec", "rema1000", "gfforsikring_tr").
var hrManagerAliasRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// NewHRManager builds the HR-Manager adapter over the given HTTP client.
func NewHRManager(c JSONGetter) Source { return hrManager{http: c} }

func (hrManager) Provider() string { return "hrmanager" }

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (hrManager) fullBoardListing() {}

type hrManagerList struct {
	CustomerAlias     string `json:"CustomerAlias"`
	TransactionStatus struct {
		StatusCode  int    `json:"StatusCode"`
		Description string `json:"Description"`
	} `json:"TransactionStatus"`
	Items               []hrManagerItem `json:"Items"`
	PositionCountSearch *int            `json:"PositionCountSearch"`
	PositionCountList   *int            `json:"PositionCountList"`
}

type hrManagerNamed struct {
	Name string `json:"Name"`
}

type hrManagerItem struct {
	ID               int64            `json:"Id"`
	Name             string           `json:"Name"`
	ProjectType      string           `json:"ProjectType"`
	AdvertisementURL string           `json:"AdvertisementUrl"`
	Published        string           `json:"Published"`
	Created          string           `json:"Created"`
	WorkPlace        string           `json:"WorkPlace"`
	Locations        []hrManagerNamed `json:"PositionLocationMultiSelection"`
	Department       *struct {
		City    string `json:"City"`
		Country string `json:"Country"`
	} `json:"Department"`
	Advertisements []struct {
		Introduction string `json:"Introduction"`
		Content      string `json:"Content"`
	} `json:"Advertisements"`
}

func (s hrManager) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(items))
	for _, it := range items {
		if it.ProjectType == hrManagerOpenApplication {
			continue
		}
		if j, ok := s.job(e, it); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// list reads the customer's whole listing in one request and proves it whole.
func (s hrManager) list(ctx context.Context, board string) ([]hrManagerItem, error) {
	if !hrManagerAliasRE.MatchString(board) {
		return nil, fmt.Errorf("hrmanager: board %q is not a customer alias", board)
	}
	q := url.Values{}
	q.Set("incads", "1")
	q.Set("take", strconv.Itoa(hrManagerTake))
	var r hrManagerList
	if err := s.http.GetJSON(ctx, fmt.Sprintf(hrManagerListURL, board)+"?"+q.Encode(), &r); err != nil {
		return nil, fmt.Errorf("hrmanager: board %q: listing: %w", board, err)
	}
	if r.TransactionStatus.StatusCode != 0 || r.PositionCountSearch == nil || r.PositionCountList == nil {
		return nil, fmt.Errorf("hrmanager: board %q: listing: status %d (%s)", board,
			r.TransactionStatus.StatusCode, strings.TrimSpace(r.TransactionStatus.Description))
	}
	// The API answering some other customer's listing would read as this board's catalogue.
	if !strings.EqualFold(r.CustomerAlias, board) {
		return nil, fmt.Errorf("hrmanager: board %q: listing names customer %q", board, r.CustomerAlias)
	}
	total, read := *r.PositionCountSearch, len(r.Items)
	switch {
	case total > hrManagerTake:
		return nil, fmt.Errorf("hrmanager: board %q lists %d postings, over the %d one request reads", board, total, hrManagerTake)
	case read != *r.PositionCountList:
		return nil, fmt.Errorf("hrmanager: board %q: read %d rows, the response says %d", board, read, *r.PositionCountList)
	case read > total || total-read > hrManagerMaxHidden:
		return nil, fmt.Errorf("hrmanager: board %q: read %d of %d postings", board, read, total)
	}
	seen := map[int64]bool{}
	for _, it := range r.Items {
		if seen[it.ID] {
			return nil, fmt.Errorf("hrmanager: board %q: posting %d listed twice", board, it.ID)
		}
		seen[it.ID] = true
	}
	return r.Items, nil
}

func (s hrManager) job(e CompanyEntry, it hrManagerItem) (Job, bool) {
	title := strings.Join(strings.Fields(it.Name), " ")
	if it.ID <= 0 || title == "" || it.AdvertisementURL == "" || e.Company == "" {
		return Job{}, false
	}
	// Every posting measured carried exactly one advertisement; the first with a body is read.
	var body string
	for _, ad := range it.Advertisements {
		if strings.TrimSpace(ad.Content) != "" {
			body = ad.Introduction + ad.Content
			break
		}
	}
	j := Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         it.AdvertisementURL,
		Title:       title,
		Company:     e.Company,
		Location:    it.location(),
		Description: sanitizeHTML(body),
		PostedAt:    hrManagerDate(it.Published),
	}
	if j.PostedAt == nil {
		j.PostedAt = hrManagerDate(it.Created)
	}
	if it.Department != nil {
		if c := location.NormalizeCountry(it.Department.Country); c != "" {
			j.Countries = []string{c}
		}
	}
	j.Remote = isRemote(j.Location)
	return j, true
}

// location is the posting's own workplace text ("Fredericia / Ballerup"), else the customer's
// location picklist, else the department's city.
func (it hrManagerItem) location() string {
	if w := strings.TrimSpace(it.WorkPlace); w != "" {
		return w
	}
	if l := distinctJoin(it.Locations, "; ", func(n hrManagerNamed) string { return strings.TrimSpace(n.Name) }); l != "" {
		return l
	}
	if it.Department != nil {
		return strings.TrimSpace(it.Department.City)
	}
	return ""
}

// hrManagerDatePattern matches the .NET JSON date the API serves every timestamp in,
// "/Date(1791377991000+0200)/" — epoch milliseconds (UTC) and the server's offset, which is
// informational only.
var hrManagerDatePattern = regexp.MustCompile(`^/Date\((-?\d+)(?:[+-]\d{4})?\)/$`)

// hrManagerDate reads one timestamp. A non-positive epoch is .NET's DateTime.MinValue — the
// platform's spelling of "never set" — and reads as absent.
func hrManagerDate(v string) *time.Time {
	m := hrManagerDatePattern.FindStringSubmatch(v)
	if m == nil {
		return nil
	}
	ms, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || ms <= 0 {
		return nil
	}
	return parseEpochMillis(ms)
}
