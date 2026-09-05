package atsapply

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// tagAutoApplyDrafting attributes drafting LLM spend to the candidate the application
// belongs to, distinct from every other feature's tag — see
// openspec/changes/auto-apply-llm-drafting/design.md's "cmd/auto-apply becomes a second
// per-user LLM caller" decision for why this worker may resolve a per-user credential at
// all. A bare word, matching internal/api/handler/user_llm.go's tag constants — the gateway's
// x-bf-dim-feature header already names the dimension, so a "feature:" prefix in the value
// would file this spend under its own two-part label instead of alongside the others.
const tagAutoApplyDrafting = "auto-apply-drafting"

// requiresCaptcha marks providers whose form always renders a captcha, so a blind fill
// attempt would either fail or (worse) look like it might work and then silently not
// submit. Lever renders one on every posting — see design.md's Risks. Every attempt for
// one of these parks before a browser is even launched.
var requiresCaptcha = map[string]bool{
	"lever": true,
}

// fillProviders is the single source of truth for which providers Submit can actually
// fill/submit for — today, Greenhouse alone (see fillAndSubmit/browser.go). Checked both
// before drafting is attempted (resolve, to avoid paying for a draft nothing can use) and
// before a fill+submit is attempted (Submit) — one set, so the two can never drift apart.
var fillProviders = map[string]bool{
	"greenhouse": true,
}

// Client drives a headless browser to resolve and, where possible, submit one application
// attempt. It implements autoapply.SidecarClient — the in-process replacement for the
// Python/Patchright sidecar the design originally proposed (see design.md's "chromedp, not
// a Python/Patchright sidecar" decision).
type Client struct {
	fetchers      map[string]applyform.Fetcher
	allocatorOpts []chromedp.ExecAllocatorOption
	// llmClient is the BASE, unbound client; nil means drafting is off (an unconfigured
	// deployment, the same convention every other LLM-backed feature follows). Bound
	// per attempt, per candidate, in Submit — never shared across attempts, the same
	// reason RunAgentAutofill binds per request rather than once at startup.
	llmClient *llm.Client
	llmKeys   *llmkey.Resolver
	// atoms is nil-checked directly (not nil-safe like llmkey.Resolver): a nil reader
	// means "no grounding source configured", and drafting is skipped entirely rather
	// than run against an always-empty GroundingContext.
	atoms AtomReader
	// cvs and renderer resolve and render a claim's approved tailored CV to a résumé PDF,
	// on demand, at submit time — no object storage involved (openspec/changes/
	// auto-apply-tailored-resume's design.md: "File rendering is on-demand"). Both nil is
	// an ordinary, supported state exactly like atoms/llmClient: a résumé field then simply
	// cannot be filled even when Resolve marked it resolvable, and attachApprovedResume
	// treats that as a render-failure park rather than a panic.
	cvs      CVReader
	renderer cv.Renderer
}

// CVReader resolves one owned CV record for rendering. *cv.Store satisfies it directly;
// tests use a fake.
type CVReader interface {
	Get(ctx context.Context, id uuid.UUID, userID int64) (cv.Record, error)
}

// NewClient builds a Client. transport is the same one internal/applyform's own capture
// worker uses (internal/sources.Client) — the Greenhouse/Ashby schema fetch this package
// reuses needs nothing different from it. llmClient/llmKeys/atoms may all be nil, which
// disables question drafting entirely and leaves every other behavior unchanged (a form
// drafting could have completed instead parks, exactly as it did before this capability
// existed). cvs/renderer may also be nil, with the same degrade: a résumé field parks
// instead of being filled.
func NewClient(transport applyform.Transport, llmClient *llm.Client, llmKeys *llmkey.Resolver, atoms AtomReader, cvs CVReader, renderer cv.Renderer) *Client {
	return &Client{
		fetchers:      applyform.Fetchers(transport),
		allocatorOpts: stealthAllocatorOptions(),
		llmClient:     llmClient,
		llmKeys:       llmKeys,
		atoms:         atoms,
		cvs:           cvs,
		renderer:      renderer,
	}
}

var _ autoapply.SidecarClient = (*Client)(nil)

// Submit resolves one attempt and submits it only when every required question is
// answered. Nothing here guesses: a field with no usable answer, or an ATS the browser
// cannot safely drive at all (a captcha board), always parks rather than risking a bad or
// duplicate submission.
func (c *Client) Submit(ctx context.Context, claimed autoapply.Claimed, answers map[string]string) (autoapply.SidecarResult, error) {
	if requiresCaptcha[claimed.Provider] {
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: "requires_captcha"}, nil
	}

	apiForm, err := c.fetchSchema(ctx, claimed)
	if err != nil {
		return autoapply.SidecarResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}

	var merged []MergedField
	var browserCtx context.Context
	var cancelBrowser context.CancelFunc

	if claimed.Provider == "greenhouse" {
		// The one provider with a live DOM-scan built so far (design.md's scope note —
		// the 2026-09-02 spike measured a real gap here; Ashby's API schema is an
		// unverified-but-accepted assumption of completeness for now).
		browserCtx, cancelBrowser, err = c.newBrowser(ctx)
		if err != nil {
			return autoapply.SidecarResult{}, fmt.Errorf("launch browser: %w", err)
		}
		defer cancelBrowser()

		pageHTML, err := renderedHTML(browserCtx, claimed.JobURL, greenhouseFormReadySelector)
		if err != nil {
			if result, parked := unscannableFormResult(err); parked {
				return result, nil
			}
			return autoapply.SidecarResult{}, fmt.Errorf("render application page: %w", err)
		}
		// A reCAPTCHA footprint can be present on a page whose known selector still
		// rendered fine — the two are independent (found by a PR review pass: the
		// check below used to run only on the OTHER branch, when the selector timed
		// out, leaving a page that scans successfully but is still challenge-gated to
		// fall through to a real fill/submit attempt with no check at all).
		if hasRecaptchaMarker(pageHTML) {
			return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: string(reasonCaptchaProtected)}, nil
		}
		dom, err := ScanGreenhouseForm(pageHTML)
		if err != nil {
			return autoapply.SidecarResult{}, fmt.Errorf("scan application form: %w", err)
		}
		merged = Reconcile(dom, apiForm)
	} else {
		merged = mergedFromAPIOnly(apiForm)
	}

	plan, err := c.resolve(ctx, claimed, merged, answers)
	if err != nil {
		return autoapply.SidecarResult{}, fmt.Errorf("resolve fields for job %d: %w", claimed.JobID, err)
	}
	if !plan.FullyResolved() {
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Unmapped: plan.Unmapped}, nil
	}

	if !fillProviders[claimed.Provider] {
		// Fill/submit is only wired for Greenhouse so far — see fill.go. A form for
		// another provider that DID fully resolve still parks rather than being
		// submitted through a path never built or verified.
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: "submission not yet implemented for this provider"}, nil
	}
	if browserCtx == nil {
		return autoapply.SidecarResult{}, fmt.Errorf("internal error: no browser session for a Greenhouse submission")
	}

	if cleanup, parked, err := c.attachApprovedResume(ctx, claimed, &plan); err != nil {
		return autoapply.SidecarResult{}, err
	} else if parked != nil {
		return *parked, nil
	} else if cleanup != nil {
		defer cleanup()
	}

	confirmed, err := fillAndSubmit(browserCtx, claimed.JobURL, plan)
	if err != nil {
		// A fill action failing, or the board EXPLICITLY refusing the submit click
		// (SUBMIT_REFUSED_MARKERS in fill.go), both mean no submission happened — safe
		// to retry normally. This is deliberately distinct from the timeout-with-no-
		// marker case below, which is NOT known to be safe to retry.
		return autoapply.SidecarResult{}, fmt.Errorf("fill and submit: %w", err)
	}
	if !confirmed {
		// Neither a confirmation nor a refusal appeared before the timeout — the click
		// may or may not have gone through. Reported as StatusUnconfirmed, not a plain
		// error: internal/autoapply's runner dead-letters this immediately rather than
		// retrying it through the ordinary attempts budget, because retrying risks a
		// second real submission to the employer.
		return autoapply.SidecarResult{Status: autoapply.StatusUnconfirmed}, nil
	}
	return autoapply.SidecarResult{Status: autoapply.StatusApplied}, nil
}

// resolve runs the deterministic pass and, where a drafting source is configured, offers
// the drafter what it left unmapped. The drafter is bound fresh for this one attempt's
// candidate (llmkey.Bind, tagged tagAutoApplyDrafting) — never shared across attempts, so
// one candidate's answers can never leak into another's LLM spend or grounding.
//
// A failure to read the grounding source degrades to "draft nothing" rather than failing
// the whole attempt: the deterministic Plan is still useful (it may already be fully
// resolved, or a partial answer is still better than none at submit time) — see the
// requirement that a non-groundable question parks rather than blocking every other field.
//
// Skipped entirely for a provider Submit cannot fill/submit for (today: everything but
// Greenhouse — see fillProviders). Found by code review: this used to run unconditionally,
// so every Ashby attempt with an unmapped field paid for a real grounding read and a real,
// budget-attributed LLM call whose answer Submit then discarded two lines later ("submission
// not yet implemented for this provider") — spend with no possible use.
func (c *Client) resolve(ctx context.Context, claimed autoapply.Claimed, merged []MergedField, answers map[string]string) (Plan, error) {
	hasApprovedCV := claimed.TailoredCVID != uuid.Nil
	if c.atoms == nil || !fillProviders[claimed.Provider] {
		return Resolve(merged, answers, hasApprovedCV), nil
	}

	grounding, err := buildGroundingContext(ctx, c.atoms, claimed.UserID)
	if err != nil {
		log.Printf("atsapply: read grounding context for user %d: %v — drafting nothing for this attempt", claimed.UserID, err)
		return Resolve(merged, answers, hasApprovedCV), nil
	}

	bound := llmkey.Bind(ctx, c.llmKeys, c.llmClient, claimed.UserID, llm.Feature(tagAutoApplyDrafting))
	return ResolveWithDrafting(ctx, merged, answers, NewLLMDrafter(bound), grounding, hasApprovedCV)
}

// attachApprovedResume renders the claim's approved tailored CV to a temp PDF and sets it
// as the résumé field's Value in plan — the file fillOne (fill.go) actually uploads.
// Cleanup removes the temp file; the caller defers it so the file outlives the fill/submit
// that reads it and is gone once Submit returns.
//
// A plan with no résumé field (nothing this attempt needs to attach) returns three nils —
// nothing to render, nothing to park, nothing to clean up.
//
// A render failure parks the attempt naming the résumé field, rather than being retried as
// a transient failure (design.md: "never guess, park instead" — the same rule an unresolved
// form field already follows). c.cvs/c.renderer being unconfigured is treated identically:
// it means this deployment cannot fill a résumé field at all yet, which is exactly the state
// a required résumé field with no known answer parks for.
func (c *Client) attachApprovedResume(ctx context.Context, claimed autoapply.Claimed, plan *Plan) (cleanup func(), parked *autoapply.SidecarResult, err error) {
	idx := -1
	for i, f := range plan.Fields {
		if f.Kind == "file" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, nil, nil
	}

	path, err := c.renderResumeToTempFile(ctx, claimed)
	if err != nil {
		log.Printf("atsapply: render approved CV %s for user %d job %d: %v", claimed.TailoredCVID, claimed.UserID, claimed.JobID, err)
		return nil, &autoapply.SidecarResult{
			Status: autoapply.StatusParked,
			Unmapped: []autoapply.UnmappedField{{
				ID: plan.Fields[idx].ID, Label: "Resume/CV", Required: true,
				Reason: "the approved tailored CV could not be rendered",
			}},
		}, nil
	}
	plan.Fields[idx].Value = path
	return func() { _ = os.Remove(path) }, nil, nil
}

// renderResumeToTempFile renders claimed's approved tailored CV through the same Typst
// renderer the interactive CV workspace's PDF download uses (GetCVPDF, internal/api/
// handler/cv.go), to a temp file chromedp can point a file input at — no object storage, no
// persistence beyond this one attempt. No photo, no traced hrefs: a machine-submitted
// résumé attachment is not the interactive preview those exist for.
func (c *Client) renderResumeToTempFile(ctx context.Context, claimed autoapply.Claimed) (string, error) {
	if c.cvs == nil || c.renderer == nil {
		return "", fmt.Errorf("no CV renderer configured")
	}
	rec, err := c.cvs.Get(ctx, claimed.TailoredCVID, claimed.UserID)
	if err != nil {
		return "", fmt.Errorf("load tailored cv %s: %w", claimed.TailoredCVID, err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return "", fmt.Errorf("resolve template %q: %w", rec.TemplateID, err)
	}
	pdf, err := c.renderer.Render(ctx, rec.Document, tmpl, nil, cv.LinkHrefs{})
	if err != nil {
		return "", fmt.Errorf("render pdf: %w", err)
	}

	f, err := os.CreateTemp("", "auto-apply-resume-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(pdf); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	return f.Name(), nil
}

// fetchSchema reuses internal/applyform's own per-provider fetcher rather than
// re-implementing Greenhouse/Ashby's API calls or Lever's page parse.
func (c *Client) fetchSchema(ctx context.Context, claimed autoapply.Claimed) (applyform.Form, error) {
	fetcher, ok := c.fetchers[claimed.Provider]
	if !ok {
		return applyform.Form{}, fmt.Errorf("no schema fetcher for provider %q", claimed.Provider)
	}
	return fetcher.Fetch(ctx, applyform.Claimed{
		JobID:      claimed.JobID,
		Provider:   claimed.Provider,
		ExternalID: claimed.ExternalID,
		URL:        claimed.JobURL,
	})
}

// unscannableFormResult reports whether err is renderedHTML's classified outcome for a form
// it could not scan — a white-label custom domain's own DOM shape, or a reCAPTCHA-gated
// form — and if so, the parked result Submit should return for it instead of a plain error.
// Neither case is a transient failure worth retrying: this parks like any other attempt
// correctly declining to guess, rather than spending the ordinary Fail/dead-letter budget on
// a form that will never change shape or stop being challenge-protected. Kept as a pure
// function, separate from Submit's real browser call, so the mapping is unit-testable
// without a live browser. See openspec/changes/auto-apply-whitelabel-greenhouse.
func unscannableFormResult(err error) (autoapply.SidecarResult, bool) {
	var unscannable *unscannableFormError
	if !errors.As(err, &unscannable) {
		return autoapply.SidecarResult{}, false
	}
	return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: string(unscannable.reason)}, true
}

// mergedFromAPIOnly builds a merged-field list straight from the platform's declared
// schema, for a provider this package has no live DOM-scan for yet. The API is trusted as
// the whole picture here — the risk Reconcile.go exists to close (a DOM-rendered field the
// API never declares) is unverified for these providers until 7.1's live smoke test.
func mergedFromAPIOnly(api applyform.Form) []MergedField {
	out := make([]MergedField, 0, len(api.Fields))
	for _, f := range api.Fields {
		if f.Type == applyform.TypeHidden || f.Type == applyform.TypeInfo {
			continue
		}
		out = append(out, MergedField{
			ID:       f.ID,
			Label:    f.Label,
			Kind:     domKindFor(f.Type),
			Required: f.Required,
			Multi:    f.Type == applyform.TypeMultiSelect,
			Options:  f.Options,
		})
	}
	return out
}

// domKindFor maps internal/applyform's platform-shaped FieldType onto this package's
// DOM-widget-shaped Kind vocabulary, for the API-only path.
func domKindFor(t applyform.FieldType) string {
	switch t {
	case applyform.TypeTextarea:
		return "textarea"
	case applyform.TypeSelect, applyform.TypeMultiSelect:
		return "select"
	case applyform.TypeFile:
		return "file"
	case applyform.TypeBoolean:
		return "checkbox_group"
	default:
		return "text"
	}
}
