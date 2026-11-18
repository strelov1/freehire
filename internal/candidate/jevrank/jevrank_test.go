package jevrank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/platform/jev"
)

// server answers each job by the title in the request state: "strong" and "partial" get
// those probabilities, "boom" a 500.
func server(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			State     map[string]string       `json:"state"`
			Questions map[string]jev.Question `json:"questions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Questions[questionKey].Type != "choice" || len(req.Questions[questionKey].Criteria) != 3 {
			t.Errorf("question = %+v", req.Questions)
		}
		job := req.State["job"]
		if seen != nil {
			*seen = append(*seen, job)
		}
		probs := map[string]float64{"strong": 0.1, "partial": 0.2, "weak": 0.7}
		choice := "weak"
		switch {
		case strings.HasPrefix(job, "boom"):
			http.Error(w, "x", http.StatusInternalServerError)
			return
		case strings.HasPrefix(job, "strong"):
			probs, choice = map[string]float64{"strong": 0.8, "partial": 0.2}, "strong"
		case strings.HasPrefix(job, "partial"):
			probs, choice = map[string]float64{"strong": 0.1, "partial": 0.8, "weak": 0.1}, "partial"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{
			questionKey: map[string]any{"choice": choice, "probabilities": probs},
		}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRankOrdersByScoreAndPutsFailuresLast(t *testing.T) {
	var seen []string
	r := New("k", jev.WithBaseURL(server(t, &seen).URL))
	got := r.Rank(context.Background(), "cv text", []Job{
		{Slug: "boom", Title: "boom"},
		{Slug: "weak", Title: "weak"},
		{Slug: "strong", Title: "strong", Description: "<p>" + strings.Repeat("x", 5000) + "</p>"},
		{Slug: "partial", Title: "partial"},
	})
	want := []struct {
		slug  string
		score float64
		label string
		ok    bool
	}{{"strong", 0.9, LabelStrong, true}, {"partial", 0.5, LabelPartial, true}, {"weak", 0.2, LabelWeak, true}, {"boom", 0, "", false}}
	if len(got) != len(want) {
		t.Fatalf("got %d results", len(got))
	}
	for i, w := range want {
		g := got[i]
		if g.Slug != w.slug || g.OK != w.ok || g.Label != w.label || g.Score < w.score-1e-9 || g.Score > w.score+1e-9 {
			t.Errorf("result %d = %+v, want %+v", i, g, w)
		}
	}
	for _, s := range seen {
		if len([]rune(s)) > maxJobRunes+200 {
			t.Errorf("job text not truncated: %d runes", len([]rune(s)))
		}
	}
}

func TestCVTextOmitsContactBlock(t *testing.T) {
	d := cv.Document{Header: cv.Header{FullName: "Ada Lovelace", Email: "ada@x.io", Phone: "123", Location: "London"}, Summary: "Engineer",
		Experience: []cv.ExperienceItem{{Role: "Dev", Company: "Acme", Bullets: []string{"Built Go services"}}}}
	text := CVText(d)
	for _, bad := range []string{"Ada", "ada@x.io", "123"} {
		if strings.Contains(text, bad) {
			t.Errorf("CV text leaks %q: %s", bad, text)
		}
	}
	for _, good := range []string{"London", "Engineer", "Dev at Acme", "Built Go services"} {
		if !strings.Contains(text, good) {
			t.Errorf("CV text lacks %q: %s", good, text)
		}
	}
}
