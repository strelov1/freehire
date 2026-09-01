package coverletter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// ErrNoPublishableEvidence means the candidate's bank holds nothing they themselves asserted.
// The chain does not run and no model is called: a letter written without evidence is exactly
// the thing this feature exists not to produce.
var ErrNoPublishableEvidence = errors.New("coverletter: no publishable evidence to draft from")

const (
	// maxCandidateRunes bounds the structured projection, as atscheck bounds its own.
	maxCandidateRunes = 12000
	// maxPostingRunes bounds the vacancy. It is the largest thing in the turn and the least
	// trusted, so it is clipped as well as stripped — the same reasoning and the same order
	// of magnitude as fitanalysis's own descriptionLimit.
	maxPostingRunes = 6000
	// maxAtomsOffered caps how much of the bank one selection stage sees. Retrieval ranks
	// before this, so the cut falls on the weakest evidence.
	maxAtomsOffered = 30
)

// Input is everything the chain needs. The vacancy arrives as a projection rather than a job
// row because this package sits in the candidate block and may not import job.
type Input struct {
	Context   fitanalysis.TailoringContext
	Candidate resumeextract.Professional
	// Atoms is the candidate's bank, UNFILTERED. Draft applies the provenance gate itself —
	// a gate a caller has to remember to apply is a convention, not a rule.
	Atoms []experience.Atom
	Band  Band
	// PostingLanguage is jobs.posting_language, verbatim and possibly empty. LanguageOf
	// decides what the letter is actually written in.
	PostingLanguage string
	Bounds          Bounds
}

// Analyzer runs the three-stage chain. A nil client (LLM unconfigured) makes Draft a no-op,
// so a server without a gateway degrades rather than errors.
type Analyzer struct {
	client *llm.Client
}

func NewAnalyzer(client *llm.Client) *Analyzer { return &Analyzer{client: client} }

// As returns an analyzer running on a different client, so one draft can be spent under the
// caller's own gateway credential. Nil-safe both ways, mirroring atscheck.Analyzer.As.
func (a *Analyzer) As(client *llm.Client) *Analyzer {
	if a == nil || client == nil {
		return a
	}
	clone := *a
	clone.client = client
	return &clone
}

// LanguageOf decides what language the letter is written in: the vacancy's, falling back to
// English when the posting carries none.
//
// This inverts matchanalysis, which writes in the CANDIDATE's profile language, and the
// inversion is the point — an analysis is the candidate reading themselves, a letter is read
// by the employer. The fallback is English rather than the profile language because about 5%
// of open postings carry no detected language, and a letter in the candidate's own tongue to
// an employer who does not read it is a mistake, where English is merely a guess.
func LanguageOf(postingLanguage string) string {
	if l := strings.TrimSpace(postingLanguage); l != "" {
		return l
	}
	return "en"
}

// Draft runs select → draft → audit and returns the audited letter.
//
// Returns (nil, nil) when the LLM is unconfigured. Returns ErrNoPublishableEvidence when the
// gate leaves nothing to write from. Any gateway failure is returned as an error with no
// letter, so a caller can never overwrite a stored draft with an empty one.
func (a *Analyzer) Draft(ctx context.Context, in Input) (*Letter, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}
	if in.Bounds == (Bounds{}) {
		in.Bounds = DefaultBounds()
	}

	// The gate runs first, before anything is marshalled. Everything downstream — the prompt,
	// the offered id set, the citation filter — is derived from this slice, so an inferred
	// atom cannot re-enter by any later route.
	atoms := Publishable(in.Atoms)
	if len(atoms) > maxAtomsOffered {
		atoms = atoms[:maxAtomsOffered]
	}
	if len(atoms) == 0 {
		return nil, ErrNoPublishableEvidence
	}
	offered := IDs(atoms)
	language := LanguageOf(in.PostingLanguage)

	selected, err := a.selectEvidence(ctx, in, atoms)
	if err != nil {
		return nil, err
	}
	// A selection stage that names nothing leaves the drafting stage with no evidence, which
	// is the empty-evidence case arriving one stage later. Fall back to what was offered
	// rather than writing from nothing.
	if len(selected) == 0 {
		selected = atoms
	}

	drafted, err := a.write(ctx, in, selected, language)
	if err != nil {
		return nil, err
	}
	drafted.Language = language
	drafted.Sanitize(in.Band, in.Bounds, offered)

	audited, err := a.audit(ctx, in, drafted)
	// The audit may improve the letter, never destroy it. An unparseable answer and one cut
	// below the floor are the same failure with different shapes, and both keep the draft.
	if err != nil || audited == nil {
		return &drafted, nil
	}
	audited.Language = language
	audited.Cited = drafted.Cited
	audited.Sanitize(in.Band, in.Bounds, offered)
	if audited.BelowFloor(in.Bounds) {
		return &drafted, nil
	}
	return audited, nil
}

// selectEvidence is stage 1: which banked achievements answer this vacancy.
func (a *Analyzer) selectEvidence(ctx context.Context, in Input, atoms []experience.Atom) ([]experience.Atom, error) {
	prompt, err := selectUserPrompt(in, atoms)
	if err != nil {
		return nil, err
	}
	raw, err := a.client.GenerateJSON(ctx, selectSystemPrompt(), prompt)
	if err != nil {
		return nil, fmt.Errorf("coverletter: select: %w", err)
	}
	var out struct {
		Selected []string `json:"selected"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		// A selection we cannot read is not fatal: the drafting stage can work from the whole
		// offered set, which is still entirely publishable evidence.
		return nil, nil
	}
	byID := make(map[uuid.UUID]experience.Atom, len(atoms))
	for _, at := range atoms {
		byID[at.ID] = at
	}
	kept := make([]experience.Atom, 0, len(out.Selected))
	for _, s := range out.Selected {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			continue
		}
		if at, ok := byID[id]; ok {
			kept = append(kept, at)
		}
	}
	return kept, nil
}

// write is stage 2: the draft itself.
func (a *Analyzer) write(ctx context.Context, in Input, atoms []experience.Atom, language string) (Letter, error) {
	prompt, err := draftUserPrompt(in, atoms, language)
	if err != nil {
		return Letter{}, err
	}
	raw, err := a.client.GenerateJSON(ctx, draftSystemPrompt(in.Band, in.Bounds), prompt)
	if err != nil {
		return Letter{}, fmt.Errorf("coverletter: draft: %w", err)
	}
	var out Letter
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return Letter{}, fmt.Errorf("coverletter: parse draft: %w", err)
	}
	out.Cited = IDs(atoms)
	return out, nil
}

// audit is stage 3: the skeptic. It returns nil when its answer cannot be used, which the
// caller reads as "keep the draft".
func (a *Analyzer) audit(ctx context.Context, in Input, drafted Letter) (*Letter, error) {
	prompt := auditUserPrompt(drafted)
	raw, err := a.client.GenerateJSON(ctx, auditSystemPrompt(in.Band, in.Bounds), prompt)
	if err != nil {
		return nil, err
	}
	var out Letter
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(out.Body) == "" {
		return nil, nil
	}
	return &out, nil
}
