// Command interview-schedule runs one pass of the opted-in Gmail scheduler and the
// Google Calendar invitation queue. It sends no messages when the rollout flag is off.
package main

import (
	"context"
	"log"
	"net/url"

	"github.com/strelov1/freehire/internal/ai/llmkey"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/interviewscheduling"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if !cfg.InterviewSchedulingEnabled || cfg.InterviewSchedulingPilotUserID < 0 {
		return worker.Skip("INTERVIEW_SCHEDULING_ENABLED")
	}
	g := cfg.OAuth["google"]
	if g.ClientID == "" || g.ClientSecret == "" || len(cfg.GmailTokenKey) != 32 || len(cfg.JWTSecret) < 32 {
		return worker.Skip("Google OAuth, GMAIL_TOKEN_KEY and JWT_SECRET")
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		log.Printf("interview-schedule: token key: %v", err)
		return 1
	}
	origin, err := url.Parse(cfg.FrontendOrigin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil {
		log.Print("interview-schedule: a public https origin is required")
		return 1
	}
	q := db.New(pool)
	connector := gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin)
	calendar := bookinglink.NewGoogleCalendar(q, connector, cipher)
	failed := 0
	if err := bookinglink.RunInvitations(ctx, bookinglink.NewInvitationQueries(q, cfg.InterviewSchedulingPilotUserID), calendar, 30); err != nil {
		log.Printf("interview-schedule: calendar: %v", err)
		failed++
	}

	model, flush, err := llm.NewClient(cfg.Settings(cfg.Model), "interview-schedule")
	if err != nil {
		log.Printf("interview-schedule: llm: %v", err)
		return 1
	}
	defer flush()
	if model == nil {
		return worker.Skip("interview scheduling LLM")
	}
	keys := llmkey.NewResolver(q, llmkey.New(llmkey.Config{
		BaseURL: cfg.LLMAdminURL, AdminUsername: cfg.LLMAdminUsername, AdminPassword: cfg.LLMAdminPassword,
		TemplateKey: cfg.LLMAdminTemplateKey, MaxBudget: cfg.LLMUserMaxBudget,
		RPMLimit: cfg.LLMUserRPMLimit, BudgetWindow: cfg.LLMUserBudgetWindow,
	}))
	plans := plan.NewStore(q, pool, plan.ConfigFromEnv())
	bookings := bookinglink.New(bookinglink.NewQueriesRepository(q, pool), plans, []byte(cfg.JWTSecret), calendar)
	var allow []int64
	if cfg.InterviewSchedulingPilotUserID > 0 {
		allow = []int64{cfg.InterviewSchedulingPilotUserID}
	}
	agent := interviewscheduling.NewWorker(q, bookings, plans, model, keys, cipher, connector, cfg.FrontendOrigin, allow)
	if err := agent.Run(ctx, 30); err != nil {
		log.Printf("interview-schedule: gmail: %v", err)
		failed++
	}
	return worker.ExitCode(failed, 0)
}
