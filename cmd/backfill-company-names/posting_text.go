package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// postingTextSamples is how many of a board's postings the model reads. One posting often
// names the employer only in a footer or not at all; three rarely all miss, and the cost is
// one call per board either way.
const postingTextSamples = 3

// postingTextLimit bounds each posting sent to the model. The employer is almost always
// named in the opening paragraph or the closing boilerplate, so the head of a long body is
// enough and the prompt stays small.
const postingTextLimit = 3000

type descriptionLister interface {
	ListPlaceholderBoardDescriptions(context.Context, db.ListPlaceholderBoardDescriptionsParams) ([]string, error)
}

type jsonModel interface {
	GenerateJSON(ctx context.Context, system, user string, opts ...llm.GenOption) (string, error)
}

// postingTextResolver names a placeholder board's employer from its stored posting text, for
// an ATS that states the employer nowhere else. ADP is the case it exists for: measured
// 2026-10-25 in a headless browser, its career center renders only a logo image with a
// random file name, and neither the listing, the requisition detail nor any of the page's
// six configuration requests names the client — while 7 of 10 sampled boards name it in the
// first lines of the posting ("Black River Health is seeking…").
//
// The model READS, it does not decide: companyname.GroundedName admits its answer only when
// the answer appears verbatim in the postings it was given, and returns the postings' own
// spelling of it. A name the model inferred, paraphrased or knew from elsewhere is refused.
type postingTextResolver struct {
	q      descriptionLister
	model  jsonModel
	source string
}

const postingTextSystem = `You read job postings and report which employer is hiring.
Reply with JSON only: {"employer": "<name>"} or {"employer": null}.
Copy the employer's name exactly as it is written in the postings — same words, same spelling.
Give the name in full, as the employer calls itself ("SNAP Diagnostics", not "SNAP"), but leave out an abbreviation in brackets after it and a legal tail such as ", A Corporation".
Use null when the postings do not name the employer, when they name more than one different employer, or when a staffing agency writes for an unnamed client.
Never infer a name from a product, a website, a location or your own knowledge.`

func (r postingTextResolver) Name(ctx context.Context, board string) (string, error) {
	rows, err := r.q.ListPlaceholderBoardDescriptions(ctx, db.ListPlaceholderBoardDescriptionsParams{
		Source:       r.source,
		BoardPattern: externalid.BoardPattern(board),
		MaxRows:      postingTextSamples,
	})
	if err != nil {
		return "", fmt.Errorf("descriptions: %w", err)
	}
	texts := make([]string, 0, len(rows))
	for _, body := range rows {
		if text := llm.TrimTruncateRunes(htmltext.ToText(body), postingTextLimit); text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return "", nil
	}

	var prompt strings.Builder
	for i, text := range texts {
		fmt.Fprintf(&prompt, "--- Posting %d ---\n%s\n\n", i+1, text)
	}
	raw, err := r.model.GenerateJSON(ctx, postingTextSystem, prompt.String())
	if err != nil {
		return "", fmt.Errorf("model: %w", err)
	}
	var out struct {
		Employer *string `json:"employer"`
	}
	if err := json.Unmarshal([]byte(llm.StripJSONFence(raw)), &out); err != nil || out.Employer == nil {
		return "", nil
	}
	name, ok := companyname.GroundedName(*out.Employer, texts)
	if !ok {
		return "", nil
	}
	return name, nil
}

// capProvider keeps at most n of one provider's rows, taken from a window that starts at
// offset and wraps, plus every other provider's — how a run bounds the boards it asks a model
// about without touching the providers that cost nothing. n <= 0 keeps everything.
//
// The window has to MOVE between runs. A board the model cannot name stays a placeholder and
// stays in the list, so a window fixed at the front fills with refusals within a few runs and
// the boards behind them are never read — the failure backfill-derive carried until
// freehire#2864. The caller passes an offset that advances by n each day, so successive runs
// sweep the whole list, and a refused board is asked again only once per sweep.
func capProvider(rows []db.ListPlaceholderCompanyBoardsRow, provider string, n, offset int64) []db.ListPlaceholderCompanyBoardsRow {
	if n <= 0 {
		return rows
	}
	var mine []int
	for i, row := range rows {
		if row.Provider == provider {
			mine = append(mine, i)
		}
	}
	if int64(len(mine)) <= n {
		return rows
	}
	// Indexes stay int64 throughout: n arrived as a 64-bit knob, and narrowing it to int is a
	// conversion nothing here needs.
	keep := make(map[int]bool)
	start := offset % int64(len(mine))
	for k := int64(0); k < n; k++ {
		keep[mine[(start+k)%int64(len(mine))]] = true
	}
	out := make([]db.ListPlaceholderCompanyBoardsRow, 0, len(rows))
	for i, row := range rows {
		if row.Provider != provider || keep[i] {
			out = append(out, row)
		}
	}
	return out
}
