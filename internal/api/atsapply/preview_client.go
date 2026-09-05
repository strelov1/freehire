package atsapply

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// StoredFormReader reads a job's previously-captured application form, when one exists.
// Preview prefers this over a live schema fetch for every provider except Greenhouse (the
// one with a live DOM scan) — see design.md's "prefer the stored apply_forms row" decision:
// cmd/capture-apply-form already persists exactly this payload, and re-fetching it live for
// every preview would cost a network round trip a stored, rarely-stale row already answers.
// May be nil, in which case Preview always fetches live instead — the same behavior Client
// itself already has.
type StoredFormReader interface {
	// GetStoredForm returns the job's captured form and true, or a zero Form and false when
	// none has been captured yet — not an error: a majority of jobs have no captured form at
	// all (JobApplyForm's own doc comment), and that is an ordinary outcome here too.
	GetStoredForm(ctx context.Context, jobID int64) (applyform.Form, bool, error)
}

// PreviewClient computes a ResolvedPreview for one auto-apply attempt: the same
// deterministic answer resolution the real submission starts from, without ever running an
// LLM draft (no spend before the candidate has approved anything — see design.md) or
// keeping a browser open past the scan it needs for Greenhouse.
//
// A separate type from Client rather than another method on it: Client is wired for
// cmd/auto-apply's own submission dependencies (LLM drafting, CV rendering) that Preview
// never touches, and its one caller (cmd/auto-apply's own new preview pass) constructs it
// independently for exactly that reason.
type PreviewClient struct {
	fetchers      map[string]applyform.Fetcher
	forms         StoredFormReader
	allocatorOpts []chromedp.ExecAllocatorOption
}

// NewPreviewClient builds a PreviewClient. forms may be nil, degrading every provider to a
// live schema fetch (fine, just one more request than necessary).
func NewPreviewClient(transport applyform.Transport, forms StoredFormReader) *PreviewClient {
	return &PreviewClient{
		fetchers:      applyform.Fetchers(transport),
		forms:         forms,
		allocatorOpts: stealthAllocatorOptions(),
	}
}

// Preview resolves what an unattended submission of claimed would currently send, without
// submitting anything. hasApprovedCV mirrors Client.resolve's own parameter: whether claimed
// carries an approved tailored CV, the one fact that lets the résumé field resolve at all.
func (p *PreviewClient) Preview(ctx context.Context, claimed autoapply.Claimed, answers map[string]string, hasApprovedCV bool) (autoapply.PreviewResult, error) {
	if requiresCaptcha[claimed.Provider] {
		// Submit will always park this provider before touching a browser or a fetcher;
		// there is nothing to preview and nothing here should pretend otherwise.
		return autoapply.PreviewResult{Parked: true, Reason: "requires_captcha"}, nil
	}

	if claimed.Provider == "greenhouse" {
		return p.previewGreenhouse(ctx, claimed, answers, hasApprovedCV)
	}

	apiForm, err := p.schemaFor(ctx, claimed)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}
	return autoapply.PreviewResult{Preview: PreviewAnswers(mergedFromAPIOnly(apiForm), answers, hasApprovedCV)}, nil
}

// schemaFor prefers a stored form over a live fetch, matching design.md's own reasoning:
// the stored row is what a live fetch would return anyway for a provider with no live
// DOM-scan path, refreshed only on a deliberate re-capture, not on every read.
func (p *PreviewClient) schemaFor(ctx context.Context, claimed autoapply.Claimed) (applyform.Form, error) {
	if p.forms != nil {
		if form, ok, err := p.forms.GetStoredForm(ctx, claimed.JobID); err != nil {
			return applyform.Form{}, err
		} else if ok {
			return form, nil
		}
	}
	fetcher, ok := p.fetchers[claimed.Provider]
	if !ok {
		return applyform.Form{}, fmt.Errorf("no schema fetcher for provider %q", claimed.Provider)
	}
	return fetcher.Fetch(ctx, applyform.Claimed{
		JobID: claimed.JobID, Provider: claimed.Provider,
		ExternalID: claimed.ExternalID, URL: claimed.JobURL,
	})
}

// previewGreenhouse scans the live form exactly the way Client.Submit does for its own
// Greenhouse branch, then closes the browser immediately — a preview never fills or
// submits, so nothing here needs the session to outlive the scan.
func (p *PreviewClient) previewGreenhouse(ctx context.Context, claimed autoapply.Claimed, answers map[string]string, hasApprovedCV bool) (autoapply.PreviewResult, error) {
	apiForm, err := p.schemaFor(ctx, claimed)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}

	browserCtx, cancel, err := newBrowserSession(ctx, p.allocatorOpts)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("launch browser: %w", err)
	}
	defer cancel()

	pageHTML, err := renderedHTML(browserCtx, claimed.JobURL, greenhouseFormReadySelector)
	if err != nil {
		if result, parked := unscannableFormResult(err); parked {
			return autoapply.PreviewResult{Parked: true, Reason: result.Reason}, nil
		}
		return autoapply.PreviewResult{}, fmt.Errorf("render application page: %w", err)
	}
	if hasRecaptchaMarker(pageHTML) {
		return autoapply.PreviewResult{Parked: true, Reason: string(reasonCaptchaProtected)}, nil
	}
	dom, err := ScanGreenhouseForm(pageHTML)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("scan application form: %w", err)
	}
	merged := Reconcile(dom, apiForm)
	return autoapply.PreviewResult{Preview: PreviewAnswers(merged, answers, hasApprovedCV)}, nil
}
