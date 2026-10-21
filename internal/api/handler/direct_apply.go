package handler

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/directapply"
)

// directApplyHandlers serve the application form of an employer vacancy that takes
// applications on freehire, and take the application. svc is nil when the deployment
// cannot deliver one (no Typst to render the CV, or no mail transport): both routes then
// answer 501 rather than recording an application nobody can receive.
type directApplyHandlers struct {
	svc *directapply.Service
}

func (h *directApplyHandlers) register(api fiber.Router, mw middleware) {
	// optionalCookie, not optional: the read carries the caller's answer-bank prefill (a
	// salary, a work permit), and optional admits any API key — a narrow `cv` or embed key
	// included — with no scope gate. The form is a browser surface, so a key loses nothing.
	api.Get("/jobs/:slug/direct-apply", publicReadLimiter(mw.public), mw.optionalCookie, h.Form)
	api.Post("/jobs/:slug/direct-apply", mw.cookie, h.Apply)
}

type directApplyQuestion struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type directApplyForm struct {
	CompanyName string                `json:"company_name"`
	Questions   []directApplyQuestion `json:"questions"`
	Open        bool                  `json:"open"`
	Applied     bool                  `json:"applied"`
	Prefill     map[string]string     `json:"prefill"`
}

// Form returns the vacancy's questions and, for a signed-in candidate, their prefill and
// whether they already applied.
func (h *directApplyHandlers) Form(c *fiber.Ctx) error {
	if h.svc == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "applications on freehire are not available")
	}
	userID, _ := auth.UserID(c)
	view, err := h.svc.Form(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return directApplyError(err)
	}
	out := directApplyForm{
		CompanyName: view.CompanyName,
		Questions:   make([]directApplyQuestion, len(view.Questions)),
		Open:        view.Open,
		Applied:     view.Applied,
		Prefill:     view.Prefill,
	}
	if out.Prefill == nil {
		out.Prefill = map[string]string{}
	}
	for i, f := range view.Questions {
		q := directApplyQuestion{ID: f.ID, Label: f.Label, Type: string(f.Type), Required: f.Required}
		for _, o := range f.Options {
			q.Options = append(q.Options, o.Value)
		}
		out.Questions[i] = q
	}
	return c.JSON(fiber.Map{"data": out})
}

type directApplyRequest struct {
	CVID    string            `json:"cv_id"`
	Answers map[string]string `json:"answers"`
	Note    string            `json:"note"`
}

// Apply sends the caller's application. Cookie-only: it mails the caller's CV to a third
// party, which no API key is minted to do.
func (h *directApplyHandlers) Apply(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.svc == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "applications on freehire are not available")
	}
	var in directApplyRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	cvID, err := uuid.Parse(in.CVID)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "choose one of your CVs")
	}
	if err := h.svc.Apply(c.Context(), userID, c.Params("slug"), directapply.Application{
		CVID: cvID, Answers: in.Answers, Note: in.Note,
	}); err != nil {
		return directApplyError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{"sent": true}})
}

// directApplyStatus is the status of each refusal; the refusal's own text is the body.
var directApplyStatus = []struct {
	err    error
	status int
}{
	{directapply.ErrNotFound, fiber.StatusNotFound},
	{directapply.ErrUnverified, fiber.StatusForbidden},
	{directapply.ErrClosed, fiber.StatusGone},
	{directapply.ErrAlreadyApplied, fiber.StatusConflict},
	{directapply.ErrInProgress, fiber.StatusConflict},
	{directapply.ErrDailyLimit, fiber.StatusTooManyRequests},
}

// directApplyError maps the use case's refusals onto statuses. A delivery failure is logged
// with its cause and answered with ErrDelivery's own text only: the transport's error can
// name the employer's address, which is not the candidate's to read, and a 502 never
// reaches Sentry, so the log line is the only trace the cause leaves.
func directApplyError(err error) error {
	switch {
	case errors.Is(err, directapply.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, strings.TrimPrefix(err.Error(), directapply.ErrInvalid.Error()+": "))
	case errors.Is(err, directapply.ErrDelivery):
		log.Printf("directapply: %v", err)
		return fiber.NewError(fiber.StatusBadGateway, directapply.ErrDelivery.Error())
	}
	for _, m := range directApplyStatus {
		if errors.Is(err, m.err) {
			return fiber.NewError(m.status, m.err.Error())
		}
	}
	return err
}

// directApplyCVs renders the CV a candidate sends with an application — the same render
// as the CV page's download, tracer links included, which exist precisely to tell a
// candidate an employer opened their application.
type directApplyCVs struct{ pdf ownedCVPDF }

func (a directApplyCVs) PDF(ctx context.Context, userID int64, id uuid.UUID) ([]byte, error) {
	b, err := a.pdf.render(ctx, userID, id)
	if errors.Is(err, cv.ErrNotFound) {
		return nil, directapply.ErrNotFound
	}
	return b, err
}

// directApplyTracker records the application in the candidate's own tracking.
type directApplyTracker struct{ tracking *jobtracking.Service }

func (t directApplyTracker) MarkApplied(ctx context.Context, userID int64, slug, source string) error {
	_, err := t.tracking.MarkApplied(ctx, userID, slug, source)
	return err
}

// directApplyMailer translates directapply's Mail into emailnotify's: the two packages
// share a layer and cannot import each other, so the translation lives here, above both.
type directApplyMailer struct {
	mailer *emailnotify.ApplicationMailer
	origin string
}

func (m directApplyMailer) SendApplication(ctx context.Context, mail directapply.Mail) error {
	answers := make([]emailnotify.ApplicationAnswer, len(mail.Answers))
	for i, a := range mail.Answers {
		answers[i] = emailnotify.ApplicationAnswer{Question: a.Label, Answer: a.Readable()}
	}
	return m.mailer.SendApplication(ctx, emailnotify.ApplicationMail{
		To:             mail.To,
		CandidateEmail: mail.CandidateEmail,
		CompanyName:    mail.CompanyName,
		JobTitle:       mail.JobTitle,
		JobURL:         strings.TrimRight(m.origin, "/") + "/jobs/" + mail.JobSlug,
		Answers:        answers,
		Note:           mail.Note,
		CV:             mail.CV,
	})
}
