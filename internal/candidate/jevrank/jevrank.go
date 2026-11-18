// Package jevrank ranks jobs against a CV with TypeSafe's Jev: one "choice" question per
// job (strong, partial or weak fit), the answer's probabilities folded into one score.
//
// In a spike on one candidate's saved vs dismissed jobs, a Jev question of this shape
// separated them at AUC 0.90, against 0.81 for the sort=match skill vector and 0.80 for
// cvmatch, at ~0.23s a call. This exact wording has not been measured yet. Its intended
// caller is the match feed's Pro re-rank (bmad initiative-job-search-assistant). The
// transport is internal/platform/jev; this file owns the question and the scoring.
package jevrank

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/jev"
)

// Labels are the three answers the question offers.
const (
	LabelStrong  = "strong"
	LabelPartial = "partial"
	LabelWeak    = "weak"
)

const (
	questionKey = "fit"

	// concurrency bounds the calls in flight for one Rank.
	concurrency = 10
	// timeout bounds a whole Rank; 50 calls at 10 in flight take about 1.5s.
	timeout = 30 * time.Second

	// maxJobRunes bounds the description one question sends; the opening names the stack
	// and the level, which is what the question reads.
	maxJobRunes = 3000
	// maxCVRunes keeps a long CV inside one question's context; the opening (summary,
	// skills, latest roles) carries most of the signal.
	maxCVRunes = 12000

	instructions = "How well does this candidate's CV fit this job? Judge the title and level, " +
		"the core skills and the years of relevant experience. Never credit a skill the CV does not state."
)

var criteria = map[string]string{
	LabelStrong:  "The CV matches the role's level and core skills; the candidate should apply.",
	LabelPartial: "The CV covers part of what the role asks for, or is a level away.",
	LabelWeak:    "The CV is in a different field or far from the role's level or skills.",
}

// Job is what a question needs of a vacancy. Description may be HTML.
type Job struct {
	Slug, Title, Company, Seniority, Description string
	Skills                                       []string
}

// Result is one job's outcome. OK is false when Jev could not judge it: Score and Label
// are then unset, never a made-up zero.
type Result struct {
	Slug  string
	Score float64
	Label string
	OK    bool
}

// Ranker asks Jev about jobs. The zero value is not usable; build one with New.
type Ranker struct{ client *jev.Client }

// New builds a ranker authenticated with apiKey; jev.WithBaseURL points it at a test server.
func New(apiKey string, opts ...jev.Option) *Ranker {
	return &Ranker{client: jev.New(apiKey, opts...)}
}

// Rank judges every job against the CV text and returns them best first; jobs Jev failed
// on follow, in their input order.
func (r *Ranker) Rank(ctx context.Context, cvText string, jobs []Job) []Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out := make([]Result, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = r.judge(ctx, cvText, j)
		}()
	}
	wg.Wait()

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].OK != out[b].OK {
			return out[a].OK
		}
		return out[a].OK && out[a].Score > out[b].Score
	})
	return out
}

func (r *Ranker) judge(ctx context.Context, cvText string, j Job) Result {
	fail := Result{Slug: j.Slug}
	if ctx.Err() != nil {
		return fail
	}
	answers, err := r.client.Ask(ctx, map[string]string{"cv": cvText, "job": jobText(j)}, map[string]jev.Question{
		questionKey: {Type: "choice", Instructions: instructions, Criteria: criteria},
	})
	if err != nil {
		return fail
	}
	p := answers[questionKey].Probabilities
	strong, okS := p[LabelStrong]
	partial, okP := p[LabelPartial]
	label := answers[questionKey].Choice
	if _, known := criteria[label]; !known || !okS || !okP || !prob(strong) || !prob(partial) {
		return fail
	}
	return Result{Slug: j.Slug, Score: strong + 0.5*partial, Label: label, OK: true}
}

func prob(v float64) bool { return v >= 0 && v <= 1 }

func jobText(j Job) string {
	return fmt.Sprintf("%s at %s\nSeniority: %s\nSkills: %s\n\n%s",
		j.Title, j.Company, j.Seniority, strings.Join(j.Skills, ", "), truncate(htmltext.ToTextFull(j.Description), maxJobRunes))
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// CVText renders the document as plain text for a question. The contact block is left out
// (a name, address or phone is not what fit is judged on and the text leaves the process);
// the location stays.
func CVText(d cv.Document) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	if d.Header.Location != "" {
		line("Location: %s", d.Header.Location)
	}
	if d.Summary != "" {
		line("Summary: %s", d.Summary)
	}
	for _, g := range d.Skills {
		line("Skills %s: %s", g.Group, strings.Join(g.Items, ", "))
	}
	for _, e := range d.Experience {
		line("Experience: %s at %s (%s) %s", e.Role, e.Company, perioddate.FormatRange(e.Start, e.End, e.Current), e.Summary)
		for _, bullet := range e.Bullets {
			line("- %s", bullet)
		}
		if len(e.Stack) > 0 {
			line("Stack: %s", strings.Join(e.Stack, ", "))
		}
	}
	for _, p := range d.Projects {
		line("Project: %s", p.Name)
		for _, bullet := range p.Bullets {
			line("- %s", bullet)
		}
	}
	for _, e := range d.Education {
		line("Education: %s %s, %s", e.Degree, e.Field, e.Institution)
	}
	for _, c := range d.Certifications {
		line("Certification: %s, %s", c.Name, c.Issuer)
	}
	for _, l := range d.Languages {
		line("Language: %s %s", l.Name, l.Level)
	}
	return truncate(b.String(), maxCVRunes)
}
