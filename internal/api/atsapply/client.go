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
	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// tagAutoApplyDrafting attributes drafting LLM spend to the candidate the application belongs
// to. A bare word like internal/api/handler/user_llm.go's tag constants: the gateway's
// x-bf-dim-feature header already names the dimension, so a "feature:" prefix would file the
// spend under its own two-part label.
const tagAutoApplyDrafting = "auto-apply-drafting"

// There is no per-provider captcha list, and reintroducing one would be a mistake: a captcha
// is a per-employer setting on a platform (Lever postings vary from none to hCaptcha), so a
// list refuses pages before anyone looks at them. Instead browser.go's hasRecaptchaMarker
// reads the page actually being applied to.

// fillProviders is the single source of truth for which providers Submit can fill and submit
// through this package's own Chrome. Checked before drafting (resolve, to avoid paying for a
// draft nothing can use) and before fill+submit (Submit), so the two cannot drift.
//
// Every entry must also have a layout (layout.go); a test asserts the containment. Empty
// today: Chrome submitted from this host's own address and lost Greenhouse's invisible
// reCAPTCHA about as often as it passed, so it moved to the browser-use executor (see
// browserUseProviders). The DOM scan still runs through this path.
var fillProviders = map[string]bool{}

// SubmittableProviders is the set other packages may read as "an application can be COMPLETED
// here without a person": fillProviders plus the browser-use providers, the latter only while
// this deployment is enforcing (in shadow mode it sends nothing).
//
// Exported because the OJCP surface publishes it as `supports_agent_submission`. Do not copy
// it: jobview.AutoApplyProviders (a fill may be ATTEMPTED) and handler's enqueue set (may be
// QUEUED) are different sets; only this one means submitted.
//
// A copy is returned so a caller cannot mutate the map this package routes on.
func SubmittableProviders() map[string]bool {
	out := make(map[string]bool, len(fillProviders)+len(browserUseProviders))
	for provider, ok := range fillProviders {
		out[provider] = ok
	}
	if browserUseEnforce() {
		for provider, ok := range browserUseProviders {
			out[provider] = out[provider] || ok
		}
	}
	return out
}

// reasonSubmissionNotImplemented is the one park reason for two gaps that both mean nothing
// here can ever act on this attempt: fillProviders excluding a provider whose schema DID
// fetch (Ashby, Workable), and errNoSchemaFetcher (Recruitee).
const reasonSubmissionNotImplemented = "submission not yet implemented for this provider"

// errNoSchemaFetcher reports that applyform.Fetchers has no entry for a provider, distinct
// from a fetcher's own call failing (an ordinary retryable error). fetchSchema and
// PreviewClient.schemaFor return it so callers park instead of retrying a provider that will
// never gain a fetcher.
var errNoSchemaFetcher = errors.New("no schema fetcher registered for this provider")

// Client drives a headless browser to resolve and, where possible, submit one application
// attempt. It implements autoapply.SidecarClient, the in-process replacement for the
// Python/Patchright sidecar.
type Client struct {
	fetchers      map[string]applyform.Fetcher
	allocatorOpts []chromedp.ExecAllocatorOption
	// llmClient is the BASE, unbound client; nil means drafting is off. Bound per attempt, per
	// candidate, in Submit, never shared across attempts.
	llmClient *llm.Client
	llmKeys   *llmkey.Resolver
	// typesafeAPIKey backs the Jev provider for select-kind drafted fields (JevSelectDrafter).
	// Empty disables it: those fields draft through the LLM.
	typesafeAPIKey string
	// atoms is nil-checked directly (unlike the nil-safe llmkey.Resolver): nil means no
	// grounding source, and drafting is skipped rather than run on an empty GroundingContext.
	atoms AtomReader
	// letters is nil-checked like atoms: nil means a cover-letter field drafts through the
	// generic Drafter.
	letters LetterReader
	// cvs and renderer render a claim's approved tailored CV to a résumé PDF on demand at
	// submit time. Both nil is a supported state: a résumé field then cannot be filled, and
	// attachApprovedResume parks the attempt instead of panicking.
	cvs      CVReader
	renderer cv.Renderer
	// browserUse executes an already-resolved Plan through the browser-use cloud agent for a
	// provider fillProviders excludes (see browseruse_fill.go). Nil is the unconfigured
	// deployment: such a form parks with reasonSubmissionNotImplemented.
	browserUse *BrowserUseExecutor
	// forms reads a job's previously-captured application form, so fetchSchema can reach it for
	// a provider with no live fetcher (today: Recruitee). Nil is the unconfigured deployment.
	forms StoredFormReader
	// entitlement answers whether the candidate is still on a paid plan before a draft call
	// spends for them. Required, not optional like the dependencies above: a nil one means
	// nobody can tell, and drafting then refuses rather than spending (resolve).
	entitlement Entitlement
}

// Entitlement is the plan question drafting asks before a model call. *plan.Store
// satisfies it; tests use a fake.
type Entitlement interface {
	RequirePaid(ctx context.Context, userID int64) (aiplan.Tier, error)
}

// reasonPaidPlanRequired parks an attempt whose candidate is no longer on a paid plan. The
// entry may wait long in the queue; a lapsed subscription buys no model call, and the attempt
// is not submitted half-answered either.
const reasonPaidPlanRequired = "the candidate's paid plan has lapsed; drafting answers needs Pro"

// WithBrowserUse attaches the browser-use fallback executor, returning c for chaining. A
// setter so NewClient's signature and call sites stay unchanged.
func (c *Client) WithBrowserUse(executor *BrowserUseExecutor) *Client {
	c.browserUse = executor
	return c
}

// WithStoredFormReader attaches the stored-form fallback fetchSchema reads before giving up on
// a provider with no live fetcher, returning c for chaining. A setter, like WithBrowserUse,
// so NewClient's signature and call sites stay unchanged.
func (c *Client) WithStoredFormReader(forms StoredFormReader) *Client {
	c.forms = forms
	return c
}

// CVReader resolves one owned CV record for rendering. *cv.Store satisfies it directly;
// tests use a fake.
type CVReader interface {
	Get(ctx context.Context, id uuid.UUID, userID int64) (cv.Record, error)
}

// NewClient builds a Client. transport is the one internal/applyform's capture worker uses.
// llmClient/llmKeys/atoms/letters may be nil, which disables question drafting and leaves
// everything else unchanged (letters alone nil only disables cover-letter reuse). The reverse
// is NOT true: resolve skips drafting, and so letter reuse, when atoms is nil. cvs/renderer
// may also be nil: a résumé field then parks instead of being filled.
// entitlement is consulted before every draft call (see resolve); unlike the others, a nil
// one does not degrade to the old behaviour — it refuses to draft.
func NewClient(transport applyform.Transport, llmClient *llm.Client, llmKeys *llmkey.Resolver, typesafeAPIKey string, atoms AtomReader, letters LetterReader, cvs CVReader, renderer cv.Renderer, entitlement Entitlement) *Client {
	return &Client{
		fetchers:       applyform.Fetchers(transport),
		allocatorOpts:  stealthAllocatorOptions(),
		llmClient:      llmClient,
		llmKeys:        llmKeys,
		typesafeAPIKey: typesafeAPIKey,
		atoms:          atoms,
		letters:        letters,
		cvs:            cvs,
		renderer:       renderer,
		entitlement:    entitlement,
	}
}

var _ autoapply.SidecarClient = (*Client)(nil)

// Submit resolves one attempt and submits it only when every required question is answered.
// Nothing here guesses: a field with no usable answer, or an ATS the browser cannot safely
// drive, parks rather than risking a bad or duplicate submission.
func (c *Client) Submit(ctx context.Context, claimed autoapply.Claimed, answers map[string]string) (autoapply.SidecarResult, error) {
	apiForm, err := c.fetchSchema(ctx, claimed)
	if err != nil {
		if errors.Is(err, errNoSchemaFetcher) {
			return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: reasonSubmissionNotImplemented}, nil
		}
		return autoapply.SidecarResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}

	var merged []MergedField
	var browserCtx context.Context
	var cancelBrowser context.CancelFunc

	// Whether this package can drive a browser at a platform is the layout registry's answer,
	// not a provider name spelled out here. A provider with no layout reaches neither a browser
	// nor a submit click, and Submit reports it as not-implemented below.
	layout, canDrive := layoutFor(claimed.Provider)

	if canDrive {
		browserCtx, cancelBrowser, err = c.newBrowser(ctx)
		if err != nil {
			return autoapply.SidecarResult{}, fmt.Errorf("launch browser: %w", err)
		}
		defer cancelBrowser()

		pageHTML, err := renderedHTML(browserCtx, layout.applyURL(claimed.JobURL), layout.formSelector)
		if err != nil {
			if result, parked := unscannableFormResult(err); parked {
				return result, nil
			}
			return autoapply.SidecarResult{}, fmt.Errorf("render application page: %w", err)
		}
		// A reCAPTCHA footprint can be present on a page whose known selector still rendered, so
		// this check runs on both paths, not only after a selector timeout.
		if hasRecaptchaMarker(pageHTML) {
			return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: string(reasonCaptchaProtected)}, nil
		}
		dom, err := ScanForm(pageHTML, layout)
		if err != nil {
			return autoapply.SidecarResult{}, fmt.Errorf("scan application form: %w", err)
		}
		merged = Reconcile(dom, apiForm)
	} else {
		merged = mergedFromAPIOnly(apiForm)
	}

	plan, err := c.resolve(ctx, claimed, merged, answers)
	if errors.Is(err, aiplan.ErrPaymentRequired) {
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: reasonPaidPlanRequired}, nil
	}
	if err != nil {
		return autoapply.SidecarResult{}, fmt.Errorf("resolve fields for job %d: %w", claimed.JobID, err)
	}
	if !plan.FullyResolved() {
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Unmapped: plan.Unmapped}, nil
	}

	if !fillProviders[claimed.Provider] {
		// browser-use fallback: for Ashby/Workable, whose schema-only Reconcile already produced
		// this fully-resolved plan, execute it through the cloud agent instead of parking, unless
		// the spend guard refuses (then the usual park). The résumé is rendered by
		// attachApprovedResume, and the executor uploads those bytes into its own workspace.
		if c.browserUse != nil && browserUseEligible(claimed.Provider, plan) {
			cleanup, parked := c.attachApprovedResume(ctx, claimed, &plan)
			if parked != nil {
				return *parked, nil
			}
			if cleanup != nil {
				defer cleanup()
			}
			if !browserUseEnforce() {
				log.Printf("atsapply: browser-use fallback would attempt job %d (%s) — shadow mode, still parking", claimed.JobID, claimed.Provider)
			} else if result, handled, err := c.browserUse.submit(ctx, plan, merged, agentTargetURL(claimed.Provider, claimed.JobURL)); handled {
				return result, err
			}
		}
		// A form for another provider that DID fully resolve still parks rather than being
		// submitted through a path never built or verified.
		return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: reasonSubmissionNotImplemented}, nil
	}
	if browserCtx == nil {
		return autoapply.SidecarResult{}, fmt.Errorf("internal error: no browser session for a submission this package can drive")
	}

	cleanup, parked := c.attachApprovedResume(ctx, claimed, &plan)
	if parked != nil {
		return *parked, nil
	}
	if cleanup != nil {
		defer cleanup()
	}

	confirmed, err := fillAndSubmit(browserCtx, claimed.JobID, plan, layout)
	if err != nil {
		if errors.Is(err, errCaptchaRefused) {
			// The board could not VERIFY the submission, i.e. no application was created. Its own
			// status because the runner retries it on a far more generous budget: an invisible
			// captcha is a coin toss and a further ask costs the employer nothing.
			return autoapply.SidecarResult{Status: autoapply.StatusCaptchaRefused, Reason: err.Error()}, nil
		}
		// A fill action failing, or the board EXPLICITLY refusing the submission
		// (SUBMIT_REFUSED_MARKERS in fill.go), both mean no submission happened: safe to retry.
		// Unlike the timeout-with-no-marker case below, which is not.
		return autoapply.SidecarResult{}, fmt.Errorf("fill and submit: %w", err)
	}
	if !confirmed {
		// Neither a confirmation nor a refusal appeared before the timeout: the click may or may
		// not have gone through. StatusUnconfirmed, not an error, because the runner dead-letters
		// it instead of retrying, which risks a second real submission.
		return autoapply.SidecarResult{Status: autoapply.StatusUnconfirmed}, nil
	}
	return autoapply.SidecarResult{Status: autoapply.StatusApplied}, nil
}

// resolve runs the deterministic pass and, where a drafting source is configured, offers the
// drafter what it left unmapped. The drafter is bound fresh for this attempt's candidate
// (llmkey.Bind, tagged tagAutoApplyDrafting), never shared across attempts, so one candidate's
// answers cannot leak into another's LLM spend or grounding.
//
// A failure to read the grounding source degrades to "draft nothing": the deterministic Plan
// is still useful and a non-groundable question parks rather than blocking every other field.
//
// Skipped entirely for a provider Submit cannot fill/submit for (see submits), so no spend goes
// on a draft Submit would discard.
func (c *Client) resolve(ctx context.Context, claimed autoapply.Claimed, merged []MergedField, answers map[string]string) (Plan, error) {
	hasApprovedCV := claimed.TailoredCVID != uuid.Nil
	// A nil c.atoms disables ResolveWithDrafting entirely, including cover-letter reuse even when
	// c.letters IS configured: atoms and letters are not independent switches. Not reachable
	// today (cmd/auto-apply/main.go wires both), so a plain deterministic Resolve.
	if c.atoms == nil || !c.submits(claimed.Provider) {
		return Resolve(merged, answers, hasApprovedCV), nil
	}
	// Nothing left to draft needs no model, and so no paid plan: a candidate whose answers
	// already cover the form is not parked over an entitlement this attempt never uses.
	if plan := Resolve(merged, answers, hasApprovedCV); plan.FullyResolved() {
		return plan, nil
	}

	grounding, err := buildGroundingContext(ctx, c.atoms, claimed.UserID)
	if err != nil {
		log.Printf("atsapply: read grounding context for user %d: %v — drafting nothing for this attempt", claimed.UserID, err)
		return Resolve(merged, answers, hasApprovedCV), nil
	}

	// Every model call made for a person needs them on a paid plan, and this worker sits behind
	// no HTTP paid gate, so it asks here per attempt: the candidate may have lapsed since
	// queueing. A free answer parks (Submit); a failed read is returned, so the attempt retries
	// rather than drafting on an unreadable entitlement.
	if err := c.requirePaid(ctx, claimed.UserID); err != nil {
		return Plan{}, err
	}
	bound := llmkey.Bind(ctx, c.llmKeys, c.llmClient, claimed.UserID, llm.Feature(tagAutoApplyDrafting))
	drafter := NewJevSelectDrafter(NewLLMDrafter(bound), c.typesafeAPIKey)
	return ResolveWithDrafting(ctx, merged, answers, drafter, grounding, hasApprovedCV, c.letters, claimed.UserID, claimed.JobID)
}

// submits reports whether Submit can complete an application on provider: through this
// package's own browser (fillProviders) or the browser-use executor when it is wired,
// enforcing and has budget left. resolve drafts only for these, so a draft is never paid for
// that nothing will send, nor withheld from one that will be sent. The budget is asked again
// at submission (BrowserUseExecutor.submit); a refusal there parks the attempt.
func (c *Client) submits(provider string) bool {
	if fillProviders[provider] {
		return true
	}
	return c.browserUse != nil && browserUseProviders[provider] && browserUseEnforce() && c.browserUse.spend.allow(true)
}

// requirePaid asks the entitlement, failing closed when none is configured.
func (c *Client) requirePaid(ctx context.Context, userID int64) error {
	if c.entitlement == nil {
		return errors.New("atsapply: no entitlement check configured; refusing to draft")
	}
	_, err := c.entitlement.RequirePaid(ctx, userID)
	return err
}

// attachApprovedResume renders the claim's approved tailored CV to a temp PDF and sets it as
// the résumé field's Value in plan, the file fillOne (fill.go) uploads. The caller defers
// cleanup so the file outlives the fill/submit and is gone once Submit returns. A plan with no
// résumé field returns two nils.
//
// There is no error return: a render failure parks the attempt naming the résumé field (never
// guess, park instead), and an unconfigured c.cvs/c.renderer is treated identically, since
// that deployment cannot fill a résumé field at all.
func (c *Client) attachApprovedResume(ctx context.Context, claimed autoapply.Claimed, plan *Plan) (cleanup func(), parked *autoapply.SidecarResult) {
	idx := -1
	for i, f := range plan.Fields {
		if f.Kind == "file" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, nil
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
		}
	}
	plan.Fields[idx].Value = path
	return func() { _ = os.Remove(path) }, nil
}

// renderResumeToTempFile renders claimed's approved tailored CV through the same Typst
// renderer as the interactive PDF download (GetCVPDF in internal/api/handler/cv.go) to a temp
// file chromedp can point a file input at. No photo, no traced hrefs: those are for the
// interactive preview.
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

// fetchSchema reuses internal/applyform's per-provider fetcher, live-fetching whenever one is
// registered. Deliberately NOT the storage-first order PreviewClient.schemaFor uses: a real
// submission must not read a possibly stale row captured for the job page's display.
//
// c.forms is reached only when a live fetch cannot answer: no fetcher registered, or a fetch
// that FAILED (a rate-limited host would otherwise fail every approved application before the
// browser). The stored row is also the one the candidate reviewed, and the fill still runs
// against the live page, which refuses a field the form no longer has. With nothing stored,
// the live fetch's own error is returned, so a transient failure stays retryable.
func (c *Client) fetchSchema(ctx context.Context, claimed autoapply.Claimed) (applyform.Form, error) {
	fetcher, ok := c.fetchers[claimed.Provider]
	if !ok {
		if form, storedOK, err := c.storedForm(ctx, claimed.JobID); err != nil || storedOK {
			return form, err
		}
		return applyform.Form{}, fmt.Errorf("%w: %q", errNoSchemaFetcher, claimed.Provider)
	}
	form, fetchErr := fetcher.Fetch(ctx, applyform.Claimed{
		JobID:      claimed.JobID,
		Provider:   claimed.Provider,
		ExternalID: claimed.ExternalID,
		URL:        claimed.JobURL,
	})
	if fetchErr == nil {
		return form, nil
	}
	stored, storedOK, err := c.storedForm(ctx, claimed.JobID)
	if err != nil || !storedOK {
		return applyform.Form{}, fetchErr
	}
	log.Printf("atsapply: live %s form fetch for job %d failed (%v) — using the stored form", claimed.Provider, claimed.JobID, fetchErr)
	return stored, nil
}

// storedForm reads the job's captured form, reporting false when no reader is wired.
func (c *Client) storedForm(ctx context.Context, jobID int64) (applyform.Form, bool, error) {
	if c.forms == nil {
		return applyform.Form{}, false, nil
	}
	return c.forms.GetStoredForm(ctx, jobID)
}

// unscannableFormResult reports whether err is renderedHTML's classified outcome for a form
// it could not scan (a white-label DOM shape or a reCAPTCHA-gated form), and if so the parked
// result Submit returns instead of a plain error. Neither case is transient, so it parks
// rather than spending the retry/dead-letter budget. Pure, so it is unit-testable without a
// browser.
func unscannableFormResult(err error) (autoapply.SidecarResult, bool) {
	var unscannable *unscannableFormError
	if !errors.As(err, &unscannable) {
		return autoapply.SidecarResult{}, false
	}
	return autoapply.SidecarResult{Status: autoapply.StatusParked, Reason: string(unscannable.reason)}, true
}

// mergedFromAPIOnly builds a merged-field list straight from the platform's declared schema,
// for a provider with no live DOM scan. The API is trusted as the whole picture, so a
// DOM-rendered field it never declares (the risk Reconcile exists to close) goes unseen here.
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
