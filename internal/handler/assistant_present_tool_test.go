package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// knownSlugs resolves exactly the slugs it was built with, so a test can state
// which vacancies exist without a database.
type knownSlugs struct {
	live []string
	got  []string
}

func (k *knownSlugs) ResolveSlugsToJobIDs(_ context.Context, slugs []string) ([]db.ResolveSlugsToJobIDsRow, error) {
	k.got = slugs
	out := []db.ResolveSlugsToJobIDsRow{}
	for i, s := range slugs {
		for _, live := range k.live {
			if s == live {
				out = append(out, db.ResolveSlugsToJobIDsRow{ID: int64(i + 1), PublicSlug: s})
			}
		}
	}
	return out, nil
}

// entries renders n well-formed entries, for the cap test.
func entries(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"slug":"job-%d","note":"worth a look"}`, i)
	}
	return strings.Join(parts, ",")
}

func TestPresentJobsToolRejectsMalformedEntries(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{"no slug", `{"jobs":[{"note":"worth a look"}]}`, "slug"},
		{"no note", `{"jobs":[{"slug":"go-dev-acme"}]}`, "note"},
		{"no entries", `{"jobs":[]}`, "at least one"},
		{"over the cap", `{"jobs":[` + entries(11) + `]}`, "10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := presentJobsTool(&knownSlugs{live: []string{"go-dev-acme"}})
			_, err := tool.Run(context.Background(), 3, json.RawMessage(tc.args))
			if err == nil {
				t.Fatal("want an error; a malformed deck must be reported to the model, not half-rendered")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to name %q so the model can correct itself", err, tc.want)
			}
		})
	}
}

func TestPresentJobsToolReportsEveryResolvedSlug(t *testing.T) {
	resolver := &knownSlugs{live: []string{"go-dev-acme", "rust-dev-acme"}}
	tool := presentJobsTool(resolver)

	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"heading":"Top matches","jobs":[{"slug":"go-dev-acme","note":"a"},{"slug":"rust-dev-acme","note":"b"}]}`))
	if err != nil {
		t.Fatalf("present_jobs: %v", err)
	}
	res, ok := out.(presentJobsResult)
	if !ok {
		t.Fatalf("result is %T, want presentJobsResult", out)
	}
	if len(res.Presented) != 2 {
		t.Errorf("presented = %v, want both slugs", res.Presented)
	}
	if len(res.Dropped) != 0 {
		t.Errorf("dropped = %v, want empty when every slug resolves", res.Dropped)
	}
	if len(resolver.got) != 2 {
		t.Errorf("resolver saw %d calls' worth of slugs (%v), want one batched lookup of both",
			len(resolver.got), resolver.got)
	}
}

func TestPresentJobsToolKeepsTheOrderTheModelChose(t *testing.T) {
	tool := presentJobsTool(&knownSlugs{live: []string{"a-job", "b-job", "c-job"}})

	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"jobs":[{"slug":"c-job","note":"1"},{"slug":"a-job","note":"2"},{"slug":"b-job","note":"3"}]}`))
	if err != nil {
		t.Fatalf("present_jobs: %v", err)
	}
	got := out.(presentJobsResult).Presented
	want := []string{"c-job", "a-job", "b-job"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("presented = %v, want %v — the deck renders in the model's ranking order", got, want)
		}
	}
}

func TestPresentJobsToolDropsUnknownSlugsAndKeepsTheRest(t *testing.T) {
	tool := presentJobsTool(&knownSlugs{live: []string{"go-dev-acme"}})

	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"jobs":[{"slug":"go-dev-acme","note":"a"},{"slug":"invented-role","note":"b"}]}`))
	if err != nil {
		t.Fatalf("one bad slug must not fail the call; the model would have to regenerate every rationale: %v", err)
	}
	res := out.(presentJobsResult)
	if len(res.Presented) != 1 || res.Presented[0] != "go-dev-acme" {
		t.Errorf("presented = %v, want only the slug that resolves", res.Presented)
	}
	if len(res.Dropped) != 1 || res.Dropped[0].Slug != "invented-role" {
		t.Fatalf("dropped = %v, want the invented slug named so the model can replace it", res.Dropped)
	}
	if res.Dropped[0].Reason == "" {
		t.Error("a dropped entry needs a reason; it is the model's only path to correcting itself")
	}
}

func TestPresentJobsToolFailsWhenNothingResolves(t *testing.T) {
	tool := presentJobsTool(&knownSlugs{live: []string{"go-dev-acme"}})

	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"jobs":[{"slug":"invented-one","note":"a"},{"slug":"invented-two","note":"b"}]}`))
	if err == nil {
		t.Fatal("want an error: there is no deck to show, and a success would read to the model as one")
	}
	for _, slug := range []string{"invented-one", "invented-two"} {
		if !strings.Contains(err.Error(), slug) {
			t.Errorf("error = %v, want it to name %q", err, slug)
		}
	}
}

func TestPresentJobsToolResultCarriesNoVacancyPayload(t *testing.T) {
	tool := presentJobsTool(&knownSlugs{live: []string{"go-dev-acme"}})

	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"heading":"Top matches","jobs":[{"slug":"go-dev-acme","note":"a fintech backend role","why_fits":["go"],"concerns":["onsite once a month"]}]}`))
	if err != nil {
		t.Fatalf("present_jobs: %v", err)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	// The receipt goes back into the model's history on every later turn. Echoing
	// the rationale it just wrote — let alone the vacancies search_jobs already
	// returned — would duplicate the session's most expensive context.
	if strings.Contains(string(payload), "fintech backend role") {
		t.Errorf("result = %s, want no echo of the model's own note", payload)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(payload, &keys); err != nil {
		t.Fatalf("result is not a JSON object: %v", err)
	}
	for k := range keys {
		if k != "presented" && k != "dropped" {
			t.Errorf("result carries %q; the receipt is slugs and reasons only", k)
		}
	}
}
