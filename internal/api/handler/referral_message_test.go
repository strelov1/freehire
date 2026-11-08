package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// referralJobReaderFake supplies a job without the database for handler branch tests.
type referralJobReaderFake struct{ job db.Job }

func (r referralJobReaderFake) GetJobBySlug(context.Context, string) (db.Job, error) {
	return r.job, nil
}

func (r referralJobReaderFake) ListJobsBySlugCode(context.Context, string) ([]db.Job, error) {
	return nil, nil
}

// emptyReferralBank has no publishable evidence, so a valid request stops before any model call.
type emptyReferralBank struct{}

func (emptyReferralBank) Retrieve(context.Context, int64, experience.Query, int) ([]experience.Match, error) {
	return nil, nil
}

func (emptyReferralBank) ListAtoms(context.Context, int64) ([]experience.Atom, error) {
	return nil, nil
}

func (emptyReferralBank) Professional(context.Context, int64, resumeextract.Structured) (resumeextract.Professional, error) {
	return resumeextract.Professional{}, nil
}

// TestReferralMessageRejectsInvalidRequests exercises request validation and access guards.
func TestReferralMessageRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name        string
		job         db.Job
		channel     string
		body        string
		missingJobs bool
		want        int
	}{
		{name: "another user's private job", job: db.Job{ID: 9, IsPrivate: true, CreatedBy: pgtype.Int8{Int64: 8, Valid: true}}, channel: "linkedin", want: fiber.StatusNotFound},
		{name: "unsupported channel", job: db.Job{ID: 9}, channel: "sms", want: fiber.StatusBadRequest},
		{name: "oversized request", body: strings.Repeat("x", 24001), want: fiber.StatusRequestEntityTooLarge},
		{name: "malformed JSON", body: `{`, want: fiber.StatusBadRequest},
		{name: "missing job reader", channel: "linkedin", missingJobs: true, want: fiber.StatusServiceUnavailable},
		{name: "empty experience bank", job: db.Job{ID: 9}, channel: "linkedin", want: fiber.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jobs jobSlugReader = referralJobReaderFake{job: tt.job}
			if tt.missingJobs {
				jobs = nil
			}
			h := newReferralMessageHandlers(jobs, emptyReferralBank{}, nil, nil, coverletter.NewAnalyzer(nil), llmBinding{}, nil)
			app := fiber.New(fiber.Config{ErrorHandler: RenderError})
			app.Post("/jobs/:slug/referral-message/stream", func(c *fiber.Ctx) error {
				c.Locals(auth.LocalsUserID, int64(7))
				return c.Next()
			}, h.Stream)
			body := tt.body
			if body == "" {
				body = `{"channel":"` + tt.channel + `","profile":"Sam is an engineer"}`
			}
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/jobs/test-job/referral-message/stream", strings.NewReader(body))
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.want)
			}
		})
	}
}
