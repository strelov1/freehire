package handler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/llm"
)

const referralDraftFailure = "The message could not be drafted. Please try again."

// No draft store or conversation store: the contact and the result live only for
// this request. The chain also disables generation tracing for referral input.
type referralMessageHandlers struct {
	jobs   jobSlugReader
	bank   letterBankPort
	resume *resume.Store
	fit    *fitanalysis.Service
	chain  *coverletter.Analyzer
	llm    llmBinding
	plans  *plan.Store
}

// newReferralMessageHandlers shares the existing job, bank, fit, model, and plan services.
func newReferralMessageHandlers(jobs jobSlugReader, bank letterBankPort, resumes *resume.Store, fit *fitanalysis.Service, chain *coverletter.Analyzer, binding llmBinding, plans *plan.Store) *referralMessageHandlers {
	return &referralMessageHandlers{jobs: jobs, bank: bank, resume: resumes, fit: fit, chain: chain, llm: binding, plans: plans}
}

// register protects drafting with both the signed-in cookie and paid-AI allowance gate.
func (h *referralMessageHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/jobs/:slug/referral-message/stream", mw.cookie, mw.paidAI(plan.FeatureCoverLetter), h.Stream)
}

// Stream drafts a request-only referral message over SSE for a paid seeker.
// The pasted profile and generated body are never written to a store or log.
func (h *referralMessageHandlers) Stream(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var contact coverletter.Referral
	if len(c.Body()) > 24000 {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "profile text is too long")
	}
	if err := c.BodyParser(&contact); err != nil || !contact.Valid() {
		return fiber.NewError(fiber.StatusBadRequest, coverletter.ErrInvalidReferral.Error())
	}
	contact.Profile = strings.TrimSpace(contact.Profile)
	if h.jobs == nil || h.bank == nil || h.chain == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "message drafting is unavailable")
	}
	job, err := jobBySlugOrCode(c.Context(), h.jobs, c.Params("slug"))
	if err != nil {
		return err
	}
	if job.IsPrivate && (!job.CreatedBy.Valid || job.CreatedBy.Int64 != userID) {
		return fiber.ErrNotFound
	}
	analysis, _ := h.fit.Optional(c.Context(), userID, job.ID)
	if analysis == nil {
		analysis = &matchanalysis.Analysis{}
	}
	tailoring := fitanalysis.ProjectTailoring(analysis, job, nil)
	atoms, err := coverletter.Gather(c.Context(), h.bank, userID, tailoring.MissingHave)
	if err != nil {
		return err
	}
	if len(coverletter.Publishable(atoms)) == 0 {
		return fiber.NewError(fiber.StatusConflict, "add or confirm an achievement in your experience bank before drafting a message")
	}
	if h.chain.ModelID() == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "message drafting is unavailable")
	}
	input := coverletter.Input{
		Context: tailoring, Atoms: atoms,
		Candidate:       candidateProfileFrom(c.Context(), h.resume, h.bank, userID),
		PostingLanguage: job.PostingLanguage, Referral: &contact,
	}
	caller, err := h.llm.caller(c.UserContext(), userID, llm.Feature(tagReferralMessage))
	if err != nil {
		return err
	}
	// No stored draft advances an attempt timestamp here: every new request spends one
	// cover-letter allowance, even when it targets the same job and contact.
	charge, refused, decision := chargeLetter(c.Context(), h.plans, userID, job.ID, uuid.NewString())
	if refused {
		return refuse(c, decision)
	}
	chain := h.chain.As(caller)
	conn := c.Context().Conn()
	sseHeaders(c)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		stream := newSSEStream(w, conn, sseWriteTimeout)
		defer recoverStream("referral: stream", func() {
			stream.event("stream_error", letterErrorEvent{Error: referralDraftFailure})
		})
		release := true
		defer func() {
			if release {
				releaseLetterCharge(h.plans, userID, charge)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), letterStreamTimeout)
		defer cancel()
		stop := stream.keepalive(sseKeepalive)
		defer stop()
		message, err := chain.DraftStream(ctx, input, func(stage string, done bool) {
			if !stream.event("stage", letterStageEvent{Stage: stage, Done: done}) {
				cancel()
			}
		})
		if err != nil || message == nil || strings.TrimSpace(message.Body) == "" {
			if !errors.Is(err, context.Canceled) {
				if err != nil {
					// Provider errors can include the prompt. Report only their type.
					log.Printf("referral: draft failed for user %d job %d: %T", userID, job.ID, err)
					reportStreamFault(fmt.Errorf("referral draft failed: %T", err))
				} else {
					log.Printf("referral: draft returned no message for user %d job %d", userID, job.ID)
					reportStreamFault(errors.New("referral draft returned no message"))
				}
				stream.event("stream_error", letterErrorEvent{Error: referralDraftFailure})
			}
			return
		}
		if stream.event("message", fiber.Map{
			"body":  message.Body,
			"cited": citedAtomsOf(ctx, h.bank, userID, message.Cited),
		}) {
			release = false
		}
	}))
	return nil
}
