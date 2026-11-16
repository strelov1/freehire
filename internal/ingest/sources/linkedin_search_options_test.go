package sources

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLinkedInSearchOptionsControlActualPageBudgetAndAgeFilter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tenants map[string]string
		pages   int
		age     string
	}{
		{"defaults", nil, 10, "r604800"},
		{"minimum", map[string]string{"pages": "1", "jobage": "1"}, 1, "r86400"},
		{"maximum", map[string]string{"pages": "100", "jobage": "30"}, 100, "r2592000"},
		{"pages only", map[string]string{"pages": "2"}, 2, "r604800"},
		{"age only", map[string]string{"jobage": "14"}, 10, "r1209600"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			getter := linkedInGetterFunc(func(_ context.Context, raw string) (*html.Node, string, error) {
				u, err := url.Parse(raw)
				if err != nil {
					return nil, "", err
				}
				var body string
				switch {
				case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
					q := u.Query()
					if q.Get("f_TPR") != tc.age || q.Get("start") != strconv.Itoa(pages) || q.Get("keywords") != "software engineer" || q.Get("location") != "Netherlands" {
						t.Errorf("unexpected search parameters: %s", raw)
					}
					pages++
					body = linkedInRelatedListing(strconv.Itoa(100 + pages))
				case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
					body = linkedInRelatedPosting(strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"))
				case strings.HasPrefix(u.Path, "/jobs/view/"):
					body = linkedInRelatedPosting(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/jobs/view/"), "/"))
				default:
					return nil, "", fmt.Errorf("unexpected URL %s", raw)
				}
				root, err := html.Parse(strings.NewReader(body))
				return root, raw, err
			})
			source := NewLinkedIn(getter).(linkedIn)
			pace := &recordingWaiter{}
			source.pace = pace
			jobs, err := source.Fetch(context.Background(), CompanyEntry{Board: "software engineer", Region: "Netherlands", Tenants: tc.tenants})
			if err != nil || len(jobs) != tc.pages || pages != tc.pages || pace.calls != 2*tc.pages+1 {
				t.Fatalf("search budget not applied: jobs=%d pages=%d paced=%d error=%v", len(jobs), pages, pace.calls, err)
			}
		})
	}
}

func TestLinkedInSearchOptionsRejectInvalidValuesBeforeAnyRequest(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"pages", ""}, {"pages", "0"}, {"pages", "-1"}, {"pages", "101"}, {"pages", "1.5"}, {"pages", "abc"}, {"pages", " 2 "}, {"pages", "+2"}, {"pages", "99999999999999999999999999"},
		{"jobage", ""}, {"jobage", "0"}, {"jobage", "-1"}, {"jobage", "31"}, {"jobage", "7days"}, {"jobage", "1.5"}, {"jobage", " 7 "}, {"jobage", "+7"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			calls := 0
			source := NewLinkedIn(linkedInGetterFunc(func(context.Context, string) (*html.Node, string, error) {
				calls++
				return nil, "", fmt.Errorf("unexpected request")
			})).(linkedIn)
			pace := &recordingWaiter{}
			source.pace = pace
			_, err := source.Fetch(context.Background(), CompanyEntry{Board: "software engineer", Region: "Netherlands", Tenants: map[string]string{tc.key: tc.value}})
			if err == nil || !strings.Contains(err.Error(), tc.key) || calls != 0 || pace.calls != 0 {
				t.Fatalf("invalid option reached transport or was ignored: error=%v requests=%d paced=%d", err, calls, pace.calls)
			}
		})
	}
}

func TestLinkedInSearchOptionsStopAtResultsEndAndUseRawCardOffsets(t *testing.T) {
	var offsets []string
	getter := linkedInGetterFunc(func(_ context.Context, raw string) (*html.Node, string, error) {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		var body string
		switch {
		case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
			offset := u.Query().Get("start")
			offsets = append(offsets, offset)
			switch offset {
			case "0":
				body = linkedInRelatedListing("101") + linkedInRelatedListing("102") + linkedInRelatedListing("102") + linkedInRelatedListing("bad")
			case "4":
				body = linkedInRelatedListing("102") + linkedInRelatedListing("103")
			case "6":
				body = "<ul></ul>"
			default:
				return nil, "", fmt.Errorf("wrong raw-card offset %s", offset)
			}
		case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
			body = linkedInRelatedPosting(strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"))
		case strings.HasPrefix(u.Path, "/jobs/view/"):
			body = linkedInRelatedPosting(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/jobs/view/"), "/"))
		default:
			return nil, "", fmt.Errorf("unexpected URL %s", raw)
		}
		root, err := html.Parse(strings.NewReader(body))
		return root, raw, err
	})
	source := NewLinkedIn(getter).(linkedIn)
	source.pace = &recordingWaiter{}
	jobs, err := source.Fetch(context.Background(), CompanyEntry{Board: "software engineer", Region: "Netherlands", Tenants: map[string]string{"pages": "100", "jobage": "30"}})
	if err != nil || len(jobs) != 3 || !reflect.DeepEqual(offsets, []string{"0", "4", "6"}) {
		t.Fatalf("listing exhaustion or raw offsets changed: jobs=%d offsets=%v error=%v", len(jobs), offsets, err)
	}
}
