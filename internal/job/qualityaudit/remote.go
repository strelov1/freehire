package qualityaudit

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// The judge's vocabulary. They are also the values remote_audit_judgements stores, so the
// table's CHECK constraints spell the same words.
const (
	ModeRemote  = "remote"
	ModeHybrid  = "hybrid"
	ModeOnsite  = "onsite"
	ModeUnclear = "unclear"

	WorldwideYes     = "yes"
	WorldwideNo      = "no"
	WorldwideUnclear = "unclear"
)

var (
	workModes  = []string{ModeRemote, ModeHybrid, ModeOnsite, ModeUnclear}
	worldwides = []string{WorldwideYes, WorldwideNo, WorldwideUnclear}
)

// descriptionLimit bounds the description sent to the judge. A posting states its arrangement
// in the header or the first paragraphs, or in a closing "location" note; the head of a long body
// carries it, and the bound is most of what decides the audit's spend.
const descriptionLimit = 6000

// RemotePosting is what the judge reads of one sampled posting tagged remote — deliberately
// nothing of our own tags.
type RemotePosting struct {
	ID          int64
	Title       string
	Location    string
	Description string
}

// Judgement is what the posting's own text says, as the judge read it and the grounding check
// let through.
type Judgement struct {
	WorkMode  string
	Worldwide string
}

// JSONModel is the slice of llm.Client the judge needs.
type JSONModel interface {
	GenerateJSON(ctx context.Context, system, user string, opts ...llm.GenOption) (string, error)
}

// judgeAnswer is the contract the model answers in.
type judgeAnswer struct {
	WorkMode       string `json:"work_mode"`
	WorkModeQuote  string `json:"work_mode_quote"`
	Worldwide      string `json:"worldwide"`
	WorldwideQuote string `json:"worldwide_quote"`
}

const judgeSystem = `You audit job postings. Read ONE posting and report only what its own text states about where the work is done.

Reply with JSON only:
{"work_mode": "remote" | "hybrid" | "onsite" | "unclear",
 "work_mode_quote": "<the exact words of the posting that say so, or empty>",
 "worldwide": "yes" | "no" | "unclear",
 "worldwide_quote": "<the exact words of the posting that say so, or empty>"}

work_mode:
- "remote": the posting says the work is done remotely, fully or by default.
- "hybrid": it says part of the work is done at an office or site (e.g. "2 days a week in the office").
- "onsite": it says the work is done at an office or site, or that remote work is not offered.
- "unclear": the posting does not say, or says contradictory things you cannot settle.
When the description states the arrangement explicitly, it outranks a bare location label.

worldwide:
- "yes": the posting says the candidate may work from any country (e.g. "work from anywhere in the world").
- "no": it limits where the candidate may live or work — to countries, a region, a city, or working hours tied to a time zone — or the work is hybrid/onsite.
- "unclear": it does not say.

Rules:
- Use "unclear" whenever the text does not state the answer. Never infer from the company, the industry, the job title, or what is typical.
- Every answer other than "unclear" needs a quote copied exactly from the posting. If you cannot quote it, answer "unclear".`

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

func judgeSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[judgeAnswer](
			llmschema.Enum("work_mode", workModes),
			llmschema.Enum("worldwide", worldwides),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("qualityaudit: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}

// JudgeRemote asks the model what p's own text says about where the work is done.
//
// The model is shown the posting's title, location and description as text — never our own
// tags, which it would anchor on — and must quote the posting for any answer but "unclear". An
// answer whose quote is not in the posting is demoted to "unclear", as is one outside the
// vocabulary or an answer that is not JSON: an audit that credits the model's guess measures the
// model, not the posting. Only a failed call is an error.
func JudgeRemote(ctx context.Context, m JSONModel, p RemotePosting) (Judgement, error) {
	s, err := judgeSchema()
	if err != nil {
		return Judgement{}, err
	}
	text := postingText(p)
	raw, err := m.GenerateJSON(ctx, judgeSystem, text, llm.WithSchema("remote_audit", s))
	if err != nil {
		return Judgement{}, fmt.Errorf("judge %d: %w", p.ID, err)
	}
	var a judgeAnswer
	if err := json.Unmarshal([]byte(llm.StripJSONFence(raw)), &a); err != nil {
		return Judgement{WorkMode: ModeUnclear, Worldwide: WorldwideUnclear}, nil
	}
	folded := fold(text)
	return Judgement{
		WorkMode:  grounded(a.WorkMode, a.WorkModeQuote, workModes, ModeUnclear, folded),
		Worldwide: grounded(a.Worldwide, a.WorldwideQuote, worldwides, WorldwideUnclear, folded),
	}, nil
}

// grounded keeps answer only when it is in the vocabulary and, unless it is the "unclear" value,
// its quote appears in the posting text.
func grounded(answer, quote string, vocab []string, unclear, foldedText string) string {
	if !slices.Contains(vocab, answer) || answer == unclear {
		return unclear
	}
	q := fold(quote)
	// Padded, so the quote must match whole words: "remote" does not ground in "remoteness".
	if q == "" || !strings.Contains(" "+foldedText+" ", " "+q+" ") {
		return unclear
	}
	return answer
}

var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// fold lowercases and reduces every run of anything but letters and digits to one space, so a
// quote that differs from the posting only in case, line breaks, punctuation or the emphasis
// markers htmltext leaves around bold text still grounds — while a quote whose WORDS are not in
// the posting does not.
func fold(s string) string {
	return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(s), " "))
}

// postingText is the prompt: the posting as a reader sees it.
func postingText(p RemotePosting) string {
	desc := llm.TrimTruncateRunes(collapseSpaces(htmltext.ToText(p.Description)), descriptionLimit)
	return fmt.Sprintf("Title: %s\nLocation: %s\nDescription:\n%s", p.Title, cmp.Or(p.Location, "(none given)"), desc)
}

var inlineSpaces = regexp.MustCompile(`[ \t]+`)

func collapseSpaces(s string) string { return inlineSpaces.ReplaceAllString(s, " ") }

// SourceWeight is a source and how many open remote postings it carries.
type SourceWeight struct {
	Source string
	Weight int64
}

// ChooseSources draws k distinct sources, each with probability proportional to its weight
// (Efraimidis–Spirakis: the k largest u^(1/w)). Weighting puts the week's judgements where
// visitors meet remote postings; drawing without replacement keeps the largest source from
// taking the whole budget. A source of weight 0 is never drawn.
func ChooseSources(weights []SourceWeight, k int, rng *rand.Rand) []SourceWeight {
	type keyed struct {
		SourceWeight
		key float64
	}
	keys := make([]keyed, 0, len(weights))
	for _, w := range weights {
		if w.Weight <= 0 {
			continue
		}
		keys = append(keys, keyed{w, math.Log(rng.Float64()) / float64(w.Weight)})
	}
	slices.SortFunc(keys, func(a, b keyed) int { return cmp.Compare(b.key, a.key) })
	out := make([]SourceWeight, 0, min(k, len(keys)))
	for _, kv := range keys[:min(k, len(keys))] {
		out = append(out, kv.SourceWeight)
	}
	return out
}
