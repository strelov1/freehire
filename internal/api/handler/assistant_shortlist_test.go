package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestNormalizeShortlistQueryPreservesConstraints(t *testing.T) {
	got, err := normalizeShortlistQuery("q=backend&work_mode=remote&skills=go,rust&skills_mode=and&skills_exclude=php&salary_min=80000&salary_max=120000&q_fields=title&hide_seen=true&former_employers=true&page=4&offset=60&limit=20&utm_source=mail")
	if err != nil {
		t.Fatal(err)
	}
	want := "former_employers=true&hide_seen=true&q=backend&q_fields=title&salary_max=120000&salary_min=80000&skills=go%2Crust&skills_exclude=php&skills_mode=and&work_mode=remote"
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestNormalizeShortlistQueryRefusesUnenforceableConditions(t *testing.T) {
	for _, query := range []string{"country=br", "salary_min=nope", "salary_min=9223372036854775808", "work_mode=remtoe", "hide_seen=maybe", "q_fields=secret", "skills_mode=banana", "posted_within_days=-2", "salary_min=1&salary_min=2", "q=%ZZ", strings.Repeat("q", 9000)} {
		t.Run(query[:min(len(query), 40)], func(t *testing.T) {
			if got, err := normalizeShortlistQuery(query); err == nil {
				t.Fatalf("accepted %q as %q", query, got)
			}
		})
	}
	if got, err := normalizeShortlistQuery(""); err != nil || got != "" {
		t.Fatalf("empty search = %q, %v", got, err)
	}
}

func TestShortlistSearchMatchesListingConstraints(t *testing.T) {
	index := &fakeSearcher{}
	h := &searchHandlers{search: index, seen: &fakeSeen{ids: []int64{11}}, appliedCompanies: &fakeAppliedCompanies{slugs: []string{"old-co"}}, formerEmployers: &fakeFormerEmployers{slugs: []string{"new-co", "watched-co"}}, watchlist: &fakeWatchlist{slugs: []string{"watched-co"}}}
	query := "q=backend&q_fields=title&salary_min=80000&skills=go,rust&skills_mode=and&skills_exclude=php&hide_seen=true&hide_applied_companies=true&former_employers=true&watchlist=true"
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/api/v1/jobs/search", func(c *fiber.Ctx) error { c.Locals(auth.LocalsUserID, int64(7)); return c.Next() }, newContract(contractHandlers{searchHandlers: h}))
	status, _ := doGet(t, app, "/api/v1/jobs/search?"+query+"&limit=20")
	if status != 200 {
		t.Fatalf("list status %d", status)
	}
	listing := index.got
	if _, err := h.runShortlistSearch(context.Background(), 7, query, 20, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index.got, listing) {
		t.Fatalf("shortlist %#v differs from list %#v", index.got, listing)
	}
}

func TestShortlistSearchDoesNotReturnDegradedOrUnscopedResults(t *testing.T) {
	for _, query := range []string{"skills=go", "hide_seen=true", "former_employers=true", "watchlist=true", "hide_applied_companies=true"} {
		t.Run(query, func(t *testing.T) {
			h := &searchHandlers{search: &fakeSearcher{err: search.ErrBadQuery}}
			if _, err := h.runShortlistSearch(context.Background(), 7, query, 20, 0); err == nil {
				t.Fatal("shortlist accepted an unenforced condition")
			}
		})
	}
}

func TestShortlistPresentationRejectsForeignCandidatesAndCapsTheTurn(t *testing.T) {
	tool := shortlistPresentTool(shortlistCandidates{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6}, 0)
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"outside","note":"good"}]}`)); err == nil {
		t.Fatal("presented a candidate outside the scoped search")
	}
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"a","note":"good"},{"slug":"b","note":"good"},{"slug":"c","note":"good"},{"slug":"d","note":"good"},{"slug":"e","note":"good"}]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"f","note":"good"}]}`)); err == nil {
		t.Fatal("bypassed the turn cap with another deck")
	}
}

func TestShortlistWithoutProfileDoesNotRequireSetup(t *testing.T) {
	h := assistantWithProfile(&fakeProfileRepo{getErr: userprofile.ErrNotFound}, nil)
	tools := h.shortlistDiscoveryTools(assistant.Session{UserID: 7, SearchContext: &assistant.SearchContext{Query: "skills=go"}}, 0)
	value, err := toolByName(t, tools, "get_profile").Run(context.Background(), 7, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	out, ok := value.(noProfileResult)
	if !ok {
		t.Fatalf("profile result: %T", value)
	}
	if strings.Contains(out.Next, "/my/profile") {
		t.Fatal("shortlisting requires profile setup")
	}
}

func TestShortlistToolCannotReplaceTheCapturedFilters(t *testing.T) {
	index := &fakeSearcher{}
	h := &assistantHandlers{search: &searchHandlers{search: index}}
	sess := assistant.Session{UserID: 7, SearchContext: &assistant.SearchContext{Query: "salary_min=80000&skills=go"}}
	tool := toolByName(t, h.shortlistDiscoveryTools(sess, 0), "search_jobs")
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{"query":"anything","filters":{}}`)); err == nil {
		t.Fatal("model could override the captured query")
	}
	if _, err := tool.Run(context.Background(), 8, json.RawMessage(`{}`)); err == nil {
		t.Fatal("foreign owner could search with another user's context")
	}
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{`skills = "go"`}, {`enrichment.salary_min >= 80000`}}
	if !reflect.DeepEqual(index.got.Filter, want) {
		t.Fatalf("captured filter lost: %#v", index.got.Filter)
	}
}

func TestShortlistRetryCountsOnlyTheCurrentPromptsSuccessfulCards(t *testing.T) {
	old, _ := assistant.EncodeToolResult("old", "present_jobs", `{"presented":["old1","old2"]}`)
	user, _ := assistant.EncodeUser("Find five vacancies")
	shown, _ := assistant.EncodeToolResult("shown", "present_jobs", `{"presented":["a","b","c","d","e"]}`)
	failed, _ := assistant.EncodeToolResult("failed", "present_jobs", `{"error":"not in scope"}`)
	count, err := shortlistShown([]assistant.Message{old, user, shown, failed})
	if err != nil || count != 5 {
		t.Fatalf("shown = %d, %v", count, err)
	}
	tool := shortlistPresentTool(shortlistCandidates{"f": 6}, count)
	if _, err := tool.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"f","note":"good"}]}`)); err == nil {
		t.Fatal("retry added a sixth card")
	}
}

func TestShortlistCanRequestSmallerPagesWithoutAllowingUndeliveredJobs(t *testing.T) {
	index := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{{ID: 1, Job: jobview.Job{PublicSlug: "a"}}, {ID: 2, Job: jobview.Job{PublicSlug: "b"}}}}}
	h := &assistantHandlers{search: &searchHandlers{search: index, descriptions: fixedDescriptions{body: strings.Repeat("description ", 4000)}}}
	sess := assistant.Session{UserID: 7, SearchContext: &assistant.SearchContext{Query: "skills=go"}}
	tools := h.shortlistDiscoveryTools(sess, 0)
	lookup := toolByName(t, tools, "search_jobs")
	if _, err := lookup.Run(context.Background(), 7, json.RawMessage(`{"limit":2}`)); err == nil {
		t.Fatal("oversized descriptions would be truncated")
	}
	show := toolByName(t, tools, "present_jobs")
	if _, err := show.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"b","note":"good"}]}`)); err == nil {
		t.Fatal("allowed a posting whose description was not delivered")
	}
	index.res.Hits = index.res.Hits[:1]
	if _, err := lookup.Run(context.Background(), 7, json.RawMessage(`{"limit":1}`)); err != nil {
		t.Fatal(err)
	}
	if index.got.Limit != 1 {
		t.Fatalf("limit = %d", index.got.Limit)
	}
	if _, err := show.Run(context.Background(), 7, json.RawMessage(`{"jobs":[{"slug":"a","note":"good"}]}`)); err != nil {
		t.Fatal(err)
	}
}
