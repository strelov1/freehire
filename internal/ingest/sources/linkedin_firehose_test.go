package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// firehoseSearches runs one Fetch and returns the query of every search request, in order.
func firehoseSearches(t *testing.T, board string, tenants map[string]string) []url.Values {
	t.Helper()
	var searches []url.Values
	getter := linkedInGetterFunc(func(_ context.Context, raw string) (*html.Node, string, error) {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		var body string
		switch {
		case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
			searches = append(searches, u.Query())
			body = linkedInRelatedListing(fmt.Sprint(100 + len(searches)))
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
	if _, err := source.Fetch(context.Background(), CompanyEntry{Board: board, Region: "Germany", Tenants: tenants}); err != nil {
		t.Fatal(err)
	}
	return searches
}

func TestLinkedInAnyKeywordOmitsTheKeywordsParameterOnBothPasses(t *testing.T) {
	searches := firehoseSearches(t, LinkedInAnyKeyword, map[string]string{"pages": "1", "easyapply_pages": "2", "jobage_hours": "6"})
	if len(searches) != 3 {
		t.Fatalf("want 1 primary + 2 Easy Apply requests, got %d", len(searches))
	}
	for i, q := range searches {
		if _, ok := q["keywords"]; ok {
			t.Errorf("search %d sent keywords: %v", i, q)
		}
		if q.Get("location") != "Germany" || q.Get("f_TPR") != "r21600" {
			t.Errorf("search %d query = %v", i, q)
		}
		if (q.Get("f_AL") == "true") != (i > 0) {
			t.Errorf("search %d f_AL = %q", i, q.Get("f_AL"))
		}
	}
}

func TestLinkedInKeywordSearchStillSendsKeywords(t *testing.T) {
	searches := firehoseSearches(t, "go developer", map[string]string{"pages": "1"})
	if len(searches) != 1 || searches[0].Get("keywords") != "go developer" {
		t.Fatalf("searches = %v", searches)
	}
}

func TestLinkedInJobAgeHoursOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tenants map[string]string
		seconds int
	}{
		{"absent keeps jobage days", map[string]string{"jobage": "2"}, 2 * 86400},
		{"default", nil, 7 * 86400},
		{"hours override days", map[string]string{"jobage": "2", "jobage_hours": "3"}, 3 * 3600},
		{"minimum", map[string]string{"jobage_hours": "1"}, 3600},
		{"maximum", map[string]string{"jobage_hours": "720"}, 720 * 3600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got, _, err := linkedInSearchOptions(tc.tenants)
			if err != nil || got != tc.seconds {
				t.Fatalf("window = %d, %v; want %d", got, err, tc.seconds)
			}
		})
	}
	for _, v := range []string{"", "0", "-1", "721", "1.5", "abc", " 3 ", "+3", "3h"} {
		if _, _, _, err := linkedInSearchOptions(map[string]string{"jobage_hours": v}); err == nil || !strings.Contains(err.Error(), "jobage_hours") {
			t.Errorf("jobage_hours=%q accepted or unnamed: %v", v, err)
		}
	}
}
