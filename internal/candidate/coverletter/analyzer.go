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
// The chain does not run and no model is called.
var ErrNoPublishableEvidence = errors.New("coverletter: no publishable evidence to draft from")

const (
	// maxCandidateRunes bounds the structured projection, as atscheck bounds its own.
	maxCandidateRunes = 12000
	// maxPostingRunes bounds the vacancy: the largest and least trusted input, so it is clipped
	// as well as stripped (as fitanalysis's descriptionLimit).
	maxPostingRunes = 6000
	// maxAtomsOffered caps how much of the bank one selection stage sees. Retrieval ranks
	// before this, so the cut falls on the weakest evidence.
	maxAtomsOffered = MaxSelectedAtoms
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
	// Referral uses the same evidence gate and bounds for a request-only message.
	Referral *Referral
}

// Analyzer runs the three-stage cover-letter chain and the one-call referral draft.
// A nil client (LLM unconfigured) makes Draft a no-op, so a server without a
// gateway degrades rather than errors.
type Analyzer struct {
	client *llm.Client
}

func NewAnalyzer(client *llm.Client) *Analyzer { return &Analyzer{client: client} }

// ModelID returns the underlying model id, so a stored draft can be stamped with the model
// that wrote it. Empty on a nil analyzer or unconfigured client. Mirrors
// resumeextract.Extractor.ModelID.
func (a *Analyzer) ModelID() string {
	if a == nil {
		return ""
	}
	return a.client.ModelID()
}

// As returns an analyzer spending as `caller`, so one draft is billed to the candidate's own
// gateway credential. Nil-safe, mirroring atscheck.Analyzer.As. It takes a caller, not a
// client, so the analyzer keeps whatever it was built with.
func (a *Analyzer) As(caller llm.Caller) *Analyzer {
	if a == nil {
		return a
	}
	clone := *a
	clone.client = a.client.AsCaller(caller)
	return &clone
}

// LanguageOf decides what language the letter is written in: the vacancy's, falling back to
// English when the posting carries none.
//
// Unlike matchanalysis (the CANDIDATE's profile language), a letter is read by the employer.
// English is the fallback because a letter in a tongue the employer does not read is a
// mistake, where English is merely a guess.
func LanguageOf(postingLanguage string) string {
	if l := strings.TrimSpace(postingLanguage); l != "" {
		return l
	}
	return "en"
}

// Stage names a step of the chain, as a progress reader sees it.
const (
	StageSelect = "select"
	StageDraft  = "draft"
	StageAudit  = "audit"
)

// Emit reports a stage starting or finishing. The chain takes minutes (three serial model
// calls), past what a proxy holds a silent response open, so progress keeps the connection
// alive; without it drafts came back as 504 from the edge.
type Emit func(stage string, done bool)

// Draft runs the chain and returns the audited letter, reporting no progress. A thin wrapper
// over DraftStream, mirroring matchanalysis.Analyze over AnalyzeStream.
func (a *Analyzer) Draft(ctx context.Context, in Input) (*Letter, error) {
	return a.DraftStream(ctx, in, func(string, bool) {})
}

// DraftStream runs select → draft → audit for cover letters. Referral messages take
// the top Gather evidence through one draft call with citations. Each path reports
// its stage boundaries to emit.
//
// Returns (nil, nil) when the LLM is unconfigured. Returns ErrNoPublishableEvidence when the
// gate leaves nothing to write from. Any gateway failure is returned as an error with no
// letter, so a caller can never overwrite a stored draft with an empty one.
func (a *Analyzer) DraftStream(ctx context.Context, in Input, emit Emit) (*Letter, error) {
	if in.Referral != nil {
		if !in.Referral.Valid() {
			return nil, ErrInvalidReferral
		}
		in.Band = BandShort
		in.Bounds = Bounds{ShortCeiling: 900, Floor: 60, MaxCited: 3}.OrDefault()
		if in.Referral.Channel == "email" {
			in.Bounds.ShortCeiling = 1400
		}
		if a != nil {
			clone := *a
			clone.client = a.client.WithoutTracing()
			a = &clone
		}
	}
	if a == nil || a.client == nil {
		return nil, nil
	}
	in.Bounds = in.Bounds.OrDefault()

	// The gate runs first. The prompt, the offered id set and the citation filter all derive
	// from this slice, so an inferred atom cannot re-enter by a later route.
	atoms := Publishable(in.Atoms)
	if len(atoms) > maxAtomsOffered {
		atoms = atoms[:maxAtomsOffered]
	}
	if len(atoms) == 0 {
		return nil, ErrNoPublishableEvidence
	}
	offered := IDs(atoms)
	language := LanguageOf(in.PostingLanguage)
	if in.Referral != nil {
		return a.draftReferral(ctx, in, atoms, language, emit)
	}

	emit(StageSelect, false)
	selected, err := a.selectEvidence(ctx, in, atoms)
	if err != nil {
		return nil, err
	}
	emit(StageSelect, true)
	// A selection that names nothing is the empty-evidence case one stage late. Fall back to
	// what was offered, but record nothing as CLAIMED: the citation list must not report a
	// fallback as chosen evidence.
	claimed := len(selected) > 0
	if !claimed {
		selected = atoms
	}

	// settle stamps a stage's output and bounds it; both stages use it so the audited letter
	// is held to the same limits as the draft.
	settle := func(l *Letter, cited []uuid.UUID) {
		l.Language = language
		l.Cited = cited
		l.Sanitize(in.Band, in.Bounds, offered)
	}

	emit(StageDraft, false)
	drafted, err := a.write(ctx, in, selected, language)
	if err != nil {
		return nil, err
	}
	emit(StageDraft, true)
	// Stage 1's selection is what the draft rests on until the audit says otherwise. If it was a
	// fallback to everything offered, no atom was claimed, so the list stays empty.
	drafted.Cited = nil
	if claimed {
		drafted.Cited = IDs(selected)
	}
	settle(&drafted, drafted.Cited)

	emit(StageAudit, false)
	audited, err := a.audit(ctx, in, drafted, selected)
	emit(StageAudit, true)
	// A gateway failure is NOT an allowed degradation. An unreadable answer keeps the draft (the
	// audit may improve a letter, never destroy it), but an answer that never arrived means the
	// skeptic did not run, and an un-audited letter must not be stored as audited: fail the draft.
	if err != nil {
		return nil, err
	}
	// Unparseable, or emptied: keep the draft.
	if audited == nil {
		return &drafted, nil
	}
	// Citations follow the AUDITED letter: an atom whose sentence was cut is no longer evidence.
	// An audit that named none does not mean the letter cites nothing, so the draft's list stands.
	cited := audited.Cited
	if len(cited) == 0 {
		cited = drafted.Cited
	}
	settle(audited, cited)
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
		// An unreadable selection is not fatal: the drafting stage can use the whole offered set.
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
	system := draftSystemPrompt(in.Band, in.Bounds)
	raw, err := a.client.GenerateJSON(ctx, system, prompt)
	if err != nil {
		return Letter{}, fmt.Errorf("coverletter: draft: %w", err)
	}
	// Decode into a type naming ONLY the key this stage asks for: junk under cited_atom_ids
	// (which this prompt never mentions) must not fail the draft, since the caller overwrites it.
	var out struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return Letter{}, fmt.Errorf("coverletter: parse draft: %w", err)
	}
	return Letter{Body: out.Body}, nil
}

// audit is stage 3: the skeptic. It returns nil when its answer cannot be used, which the
// caller reads as "keep the draft".
func (a *Analyzer) audit(ctx context.Context, in Input, drafted Letter, atoms []experience.Atom) (*Letter, error) {
	prompt, err := auditUserPrompt(drafted, atoms)
	if err != nil {
		return nil, err
	}
	system := auditSystemPrompt(in.Band, in.Bounds)
	raw, err := a.client.GenerateJSON(ctx, system, prompt)
	if err != nil {
		return nil, fmt.Errorf("coverletter: audit: %w", err)
	}
	// (nil, nil) means "keep the draft", for output that arrived and was unusable; it must never
	// stand in for output that never arrived.
	var out Letter
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil, nil
	}
	if strings.TrimSpace(out.Body) == "" {
		return nil, nil
	}
	return &out, nil
}
