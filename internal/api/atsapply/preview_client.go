package atsapply

import (
	"context"
	"errors"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// StoredFormReader reads a job's previously captured application form. Preview prefers it
// over a live schema fetch for every provider except Greenhouse (which has a live DOM
// scan), since cmd/capture-apply-form already persists the payload. May be nil, in which
// case Preview always fetches live.
type StoredFormReader interface {
	// GetStoredForm returns the job's captured form and true, or a zero Form and false when
	// none was captured; that is ordinary, not an error.
	GetStoredForm(ctx context.Context, jobID int64) (applyform.Form, bool, error)
}

// PreviewClient computes a ResolvedPreview for one auto-apply attempt: the deterministic
// answer resolution a real submission starts from, with no LLM draft (no spend before the
// candidate approves) and no browser kept open past the Greenhouse scan.
//
// Separate from Client because Client carries submission dependencies (LLM drafting, CV
// rendering) that Preview never touches.
type PreviewClient struct {
	fetchers      map[string]applyform.Fetcher
	forms         StoredFormReader
	allocatorOpts []chromedp.ExecAllocatorOption
}

// NewPreviewClient builds a PreviewClient. forms may be nil, degrading every provider to a
// live schema fetch.
func NewPreviewClient(transport applyform.Transport, forms StoredFormReader) *PreviewClient {
	return &PreviewClient{
		fetchers:      applyform.Fetchers(transport),
		forms:         forms,
		allocatorOpts: stealthAllocatorOptions(),
	}
}

// Preview resolves what an unattended submission of claimed would currently send, without
// submitting anything. Satisfies autoapply.PreviewSidecar.
func (p *PreviewClient) Preview(ctx context.Context, claimed autoapply.Claimed, answers map[string]string) (autoapply.PreviewResult, error) {
	// Mirrors Client.resolve's hasApprovedCV derivation. ClaimAutoApplyPreviewBatch's WHERE
	// (tailored_cv_id IS NOT NULL) guarantees it is never the zero value here.
	hasApprovedCV := claimed.TailoredCVID != uuid.Nil

	// Same registry as the submission path: the preview must scan the DOM the submission
	// will, or the candidate approves answers the fill never sees.
	if _, canDrive := layoutFor(claimed.Provider); canDrive {
		return p.previewByLayout(ctx, claimed, answers, hasApprovedCV)
	}

	apiForm, err := p.schemaFor(ctx, claimed)
	if err != nil {
		if errors.Is(err, errNoSchemaFetcher) {
			return autoapply.PreviewResult{Parked: true, Reason: reasonSubmissionNotImplemented}, nil
		}
		return autoapply.PreviewResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}
	return autoapply.PreviewResult{Preview: PreviewAnswers(mergedFromAPIOnly(apiForm), answers, hasApprovedCV)}, nil
}

// schemaFor prefers a stored form over a live fetch: for a provider with no live DOM scan
// the stored row is what a fetch would return, refreshed only on re-capture.
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
		return applyform.Form{}, fmt.Errorf("%w: %q", errNoSchemaFetcher, claimed.Provider)
	}
	return fetcher.Fetch(ctx, applyform.Claimed{
		JobID: claimed.JobID, Provider: claimed.Provider,
		ExternalID: claimed.ExternalID, URL: claimed.JobURL,
	})
}

// previewByLayout scans the live form exactly as Client.Submit does, with the same layout
// the submission will use, then closes the browser at once; a preview never fills or
// submits.
func (p *PreviewClient) previewByLayout(ctx context.Context, claimed autoapply.Claimed, answers map[string]string, hasApprovedCV bool) (autoapply.PreviewResult, error) {
	apiForm, err := p.schemaFor(ctx, claimed)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("fetch %s schema: %w", claimed.Provider, err)
	}

	browserCtx, cancel, err := newBrowserSession(ctx, p.allocatorOpts)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("launch browser: %w", err)
	}
	defer cancel()

	layout, canDrive := layoutFor(claimed.Provider)
	if !canDrive {
		// Unreachable (Preview only gets here for a drivable platform), but a discarded lookup
		// failure would let a later caller scan with a zero-valued layout.
		return autoapply.PreviewResult{}, fmt.Errorf("no form layout for %q", claimed.Provider)
	}
	pageHTML, err := renderedHTML(browserCtx, layout.applyURL(claimed.JobURL), layout.formSelector)
	if err != nil {
		if result, parked := unscannableFormResult(err); parked {
			return autoapply.PreviewResult{Parked: true, Reason: result.Reason}, nil
		}
		return autoapply.PreviewResult{}, fmt.Errorf("render application page: %w", err)
	}
	if hasRecaptchaMarker(pageHTML) {
		return autoapply.PreviewResult{Parked: true, Reason: string(reasonCaptchaProtected)}, nil
	}
	dom, err := ScanForm(pageHTML, layout)
	if err != nil {
		return autoapply.PreviewResult{}, fmt.Errorf("scan application form: %w", err)
	}
	merged := Reconcile(dom, apiForm)
	return autoapply.PreviewResult{Preview: PreviewAnswers(merged, answers, hasApprovedCV)}, nil
}
