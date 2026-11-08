package handler

import (
	"bufio"
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// No draft store or conversation store: the contact and the result live only for
// this request. The chain also disables generation tracing for referral input.
type referralMessageHandlers struct {
	jobs   jobSlugReader
	bank   letterBankPort
	resume *resume.Store
	fit    *fitanalysis.Service
	chain  *coverletter.Analyzer
	llm    llmBinding
}

func newReferralMessageHandlers(jobs jobSlugReader, bank letterBankPort, resumes *resume.Store, fit *fitanalysis.Service, chain *coverletter.Analyzer, binding llmBinding) *referralMessageHandlers {
	return &referralMessageHandlers{jobs: jobs, bank: bank, resume: resumes, fit: fit, chain: chain, llm: binding}
}

func (h *referralMessageHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/jobs/:slug/referral-message/stream", mw.cookie, mw.paidAI(unmeteredAI), h.Stream)
}

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
	if h.jobs == nil || h.bank == nil || h.chain == nil || h.chain.ModelID() == "" {
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
	input := coverletter.Input{
		Context: tailoring, Atoms: atoms,
		Candidate:       candidateProfileFrom(c.Context(), h.resume, h.bank, userID),
		PostingLanguage: job.PostingLanguage, Referral: &contact,
	}
	caller, err := h.llm.caller(c.UserContext(), userID, llm.Feature(tagReferralMessage))
	if err != nil {
		return err
	}
	chain := h.chain.As(caller)
	conn := c.Context().Conn()
	sseHeaders(c)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		stream := newSSEStream(w, conn, sseWriteTimeout)
		// Provider errors can echo a prompt. Never log them or expose them to the client.
		defer func() {
			if recover() != nil {
				stream.event("stream_error", letterErrorEvent{Error: "The message could not be drafted. Please try again."})
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
				stream.event("stream_error", letterErrorEvent{Error: "The message could not be drafted. Please try again."})
			}
			return
		}
		stream.event("message", fiber.Map{
			"body":  message.Body,
			"cited": citedAtomsOf(ctx, h.bank, userID, message.Cited),
		})
	}))
	return nil
}
