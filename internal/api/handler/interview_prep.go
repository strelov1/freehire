package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
)

type interviewPrepHandlers struct {
	jobs *jobsHandlers
	bank *experience.Store
}

func registerInterviewPrep(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/jobs/:slug/interview-prep", mw.optionalCookie, func(c *fiber.Ctx) error {
		// The same URL can return private examples depending on the session.
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		return c.Next()
	}, contract)
}

func (h *interviewPrepHandlers) GetInterviewPrep(ctx context.Context, params apigen.GetInterviewPrepParams) (*apigen.GetInterviewPrepOK, error) {
	userID, signedIn := auth.UserIDFromContext(ctx)
	posting, err := jobBySlugOrCode(ctx, h.jobs.queries, params.Slug)
	if err != nil {
		return nil, err
	}
	if posting.IsPrivate || (posting.ClosedAt.Valid && !signedIn) {
		return nil, pgx.ErrNoRows
	}
	// A posting may close before an interview. Only the account interviewing for
	// that posting can prepare from it once it leaves the open catalogue.
	if posting.ClosedAt.Valid {
		stage, err := h.jobs.queries.GetApplicationStage(ctx, db.GetApplicationStageParams{
			UserID: userID,
			JobID:  pgtype.Int8{Int64: posting.ID, Valid: true},
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if !stage.Valid || stage.String != "interview" {
			return nil, pgx.ErrNoRows
		}
	}
	job, err := jobview.FromRow(posting)
	if err != nil {
		return nil, err
	}
	questions, evidenceQuery := interviewQuestions(job)
	sheet := apigen.InterviewPrepSheet{
		Job: apigen.ToolJob{
			PublicSlug: posting.PublicSlug, Title: posting.Title, Company: posting.Company, Location: optString(posting.Location),
		},
		Questions: questions,
	}
	if signedIn {
		// Retrieve also ranks unconfirmed inferences for the assistant. Filter those
		// before applying the limit so they cannot crowd out confirmed examples.
		matches, err := h.bank.Retrieve(ctx, userID, evidenceQuery, 0)
		if err != nil {
			observability.CaptureError(ctx, err)
			sheet.ExamplesUnavailable = apigen.NewOptBool(true)
		} else {
			sheet.Examples = interviewExamples(matches)
		}
	}
	return &apigen.GetInterviewPrepOK{Data: sheet}, nil
}

func interviewQuestions(job jobview.Job) ([]apigen.InterviewPracticeQuestion, experience.Query) {
	const maxTopics = 3
	questions := make([]apigen.InterviewPracticeQuestion, 0, maxTopics+1)
	query := experience.Query{Skills: job.Skills}

	topics := make(map[string]bool)
	coveredSkills := make(map[string]bool)
	for _, requirement := range job.Enrichment.Requirements {
		topic := strings.Join(strings.Fields(requirement.Text), " ")
		key := strings.ToLower(topic)
		if topic == "" || topics[key] {
			continue
		}
		topics[key] = true
		questions = append(questions, apigen.InterviewPracticeQuestion{
			Topic:  apigen.NewOptString(topic),
			Prompt: "What example would best show how you meet this requirement? What did you personally contribute?",
		})
		query.Text = strings.TrimSpace(query.Text + " " + topic)
		for _, skill := range skilltag.Parse(topic) {
			coveredSkills[skill] = true
		}
		if len(questions) == maxTopics {
			break
		}
	}
	for _, skill := range job.Skills {
		if len(questions) == maxTopics {
			break
		}
		if coveredSkills[skill] {
			continue
		}
		questions = append(questions, apigen.InterviewPracticeQuestion{
			Topic:  apigen.NewOptString(skilltag.Label(skill)),
			Prompt: "If you’ve used this skill, describe a specific problem you solved with it. How did you know your approach worked?",
		})
	}
	if query.Text == "" && len(query.Skills) == 0 {
		query.Text = job.Title
	}
	questions = append(questions, apigen.InterviewPracticeQuestion{
		Prompt: "Which project best prepares you for the " + job.Title + " role, and what was your contribution?",
	})
	if len(questions) < 3 {
		questions = append(questions, apigen.InterviewPracticeQuestion{
			Prompt: "What would you ask the team to understand what success looks like in this role?",
		})
	}
	if len(questions) < 3 {
		questions = append(questions, apigen.InterviewPracticeQuestion{
			Prompt: "Tell me about a trade-off you faced on a project relevant to this role. What did you choose, and why?",
		})
	}
	return questions, query
}

func interviewExamples(matches []experience.Match) []apigen.InterviewPracticeExample {
	const maxExamples = 3
	examples := make([]apigen.InterviewPracticeExample, 0, maxExamples)
	for _, match := range matches {
		atom := match.Atom
		if !atom.Provenance.Publishable() {
			continue
		}
		example := apigen.InterviewPracticeExample{Claim: atom.Claim, Context: optString(atom.Context), Metrics: atom.Metrics}
		if place := match.Employment; place != nil {
			example.Place = optString(strings.Trim(strings.Join([]string{place.Role, place.Company}, " · "), " ·"))
		}
		examples = append(examples, example)
		if len(examples) == maxExamples {
			break
		}
	}
	return examples
}
