// Command cv-lifecycle-mail is the CV lifecycle letters worker. One run observes who has
// started paying, enrolls accounts whose CV has just parsed, and sends each due letter (up
// to three, on days 1, 6 and 13 after the upload) to a free user who has not paid or
// unsubscribed. The same run sends the paywall letter, one letter about an hour after a
// free user pressed a paid AI button, ahead of the CV letters. Run it on a schedule (hourly
// is plenty) and it exits. The rules and the reports are in
// internal/engage/cvlifecycle/AGENTS.md.
//
// It needs the SES transport (AWS_REGION + NOTIFY_EMAIL_FROM) and ONBOARDING_REPLY_TO, the
// human inbox that answers these letters; without the latter it refuses to run, since the
// letters invite a reply. CV_LETTER_PROMO_CODE is the code letter 3 offers: unset, letter 3
// is skipped and the other two go out. The code is read here and held in memory; it is
// never stored or logged. CV storage (S3_*) and MEILI_URL / MEILI_MASTER_KEY feed the ATS
// score the first two letters state, and a letter whose score cannot be computed stays
// pending rather than going out without it.
//
// Flags:
//
//	-dry-run                 count what a pass would do; writes nothing, sends nothing
//	-to ADDR -to-user ID     send ONE letter for inspection and exit; touches no table
//	-step N                  which letter -to sends (1, 2, 3 or 4, the paywall letter); default 1
//
// The preview of letters 1 and 2 computes the account's real score, so it needs the same CV
// and search configuration as a pass; letter 3 needs the promo code; letter 4 is a fixed
// sample, since it is about a refused attempt rather than the CV.
//
// Environment: CV_LIFECYCLE_MAX_PER_RUN (default 200) bounds one run's letters.
// It exits non-zero when a send failed or an input could not be read, so the timer's
// failure handling surfaces a broken sender rather than the sequence quietly stopping.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/cvlifecycle"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/identity/promo"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/blobstore"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

var (
	dryRun   = flag.Bool("dry-run", false, "count what a pass would do and exit; nothing is written or sent")
	sendTo   = flag.String("to", "", "send one letter to this address and exit (no ledger, no claim)")
	sendUser = flag.Int64("to-user", 0, "the account id -to belongs to; required with -to, the letter carries that account's unsubscribe link and score")
	sendStep = flag.Int("step", 1, "which letter -to sends: 1 | 2 | 3 | 4")
)

func main() {
	flag.Parse()
	worker.Main(run, emailnotify.HoldKeys("cv-lifecycle-mail")...)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("cv-lifecycle-mail: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	// A missing reply address is a misconfiguration, not an absent feature: the letters
	// invite a reply, and one that goes nowhere is worse than none.
	if cfg.OnboardingReplyTo == "" {
		log.Print("cv-lifecycle-mail: ONBOARDING_REPLY_TO is unset — refusing to send letters that ask for a reply")
		return 1
	}
	if cfg.MeiliURL == "" {
		log.Print("cv-lifecycle-mail: MEILI_URL is unset — the ATS score cannot be computed, refusing to run")
		return 1
	}

	blobs, err := blobstore.New(blobstore.Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		log.Printf("cv-lifecycle-mail: blobstore: %v", err)
		return 1
	}
	queries := db.New(pool)
	resumes := resume.New(blobs, resume.NewQueriesRepository(queries))
	if !resumes.Enabled() {
		log.Print("cv-lifecycle-mail: CV storage (S3_*) is not configured — the ATS score cannot be computed, refusing to run")
		return 1
	}
	content := cvlifecycle.NewContent(
		resumes,
		userprofile.New(userprofile.NewQueriesRepository(queries)),
		search.NewClient(cfg.MeiliURL, cfg.MeiliKey),
		promo.New(promo.NewQueriesRepository(queries), promo.ConfigFromEnv()),
		os.Getenv("CV_LETTER_PROMO_CODE"),
	)

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("cv-lifecycle-mail: ses: %v", err)
		return 1
	}
	links := emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin)

	// A single letter, for looking at it in a real client. It goes out untracked, so it
	// cannot count toward a campaign's rates, and touches neither the ledger nor the claim.
	if *sendTo != "" {
		return preview(ctx, content, cvlifecycle.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin, links))
	}

	runnerCfg := cvlifecycle.DefaultConfig()
	if runnerCfg.MaxPerRun, err = worker.EnvInt32("CV_LIFECYCLE_MAX_PER_RUN", runnerCfg.MaxPerRun); err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}

	trackedSES := ses.WithSendingBreaker().WithEngagementTracking(cfg.SESEngagementConfigurationSet)
	mailer := cvlifecycle.NewMailer(trackedSES, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin, links)
	runner, err := cvlifecycle.New(queries, content, mailer, runnerCfg)
	if err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}

	if *dryRun {
		stats, err := runner.DryRun(ctx)
		if err != nil {
			log.Printf("cv-lifecycle-mail: %v", err)
			return 1
		}
		log.Printf("cv-lifecycle-mail: dry run: %d accounts would enroll; due letters 1=%d 2=%d 3=%d paywall=%d (cap %d per run)",
			stats.Enrollable, stats.Due[cvlifecycle.StepATS], stats.Due[cvlifecycle.StepPotential], stats.Due[cvlifecycle.StepOffer], stats.Due[cvlifecycle.StepPaywall], runnerCfg.MaxPerRun)
		return 0
	}

	// Before anything goes out: a failed send burns its step, so an unknown configuration
	// set would spend the audience on rejected sends.
	if err := trackedSES.CheckEngagementTracking(ctx); err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}
	log.Printf("cv-lifecycle-mail: enrolled=%d paid=%d entitled=%d unsubscribed=%d pruned_claims=%d",
		stats.Enrolled, stats.Paid, stats.Entitled, stats.Unsubscribed, stats.Pruned)
	failed := 0
	for _, step := range append([]cvlifecycle.Step{cvlifecycle.StepPaywall}, cvlifecycle.Steps...) {
		log.Printf("cv-lifecycle-mail: letter %d sent=%d failed=%d skipped=%d deferred=%d",
			step, stats.Sent[step], stats.Failed[step], stats.Skipped[step], stats.Deferred[step])
		failed += stats.Failed[step]
	}
	if stats.Paused {
		log.Print("cv-lifecycle-mail: SES paused sending for the account; the pass stopped early and nothing was recorded for the rest")
	}
	if failed > 0 || stats.Errors > 0 {
		return 1
	}
	return 0
}

// preview composes one letter for the named account and sends it to -to.
func preview(ctx context.Context, content *cvlifecycle.Content, mailer *cvlifecycle.Mailer) int {
	if *sendUser <= 0 {
		log.Print("cv-lifecycle-mail: -to also needs -to-user, the account id the address belongs to")
		return 2
	}
	step := cvlifecycle.Step(*sendStep)
	letter, err := composePreview(ctx, content, step, *sendUser)
	if errors.Is(err, cvlifecycle.ErrSkip) {
		log.Printf("cv-lifecycle-mail: letter %d does not apply to user %d: %v", step, *sendUser, err)
		return 1
	}
	if err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}
	if err := mailer.Send(ctx, letter, *sendUser, *sendTo); err != nil {
		log.Printf("cv-lifecycle-mail: %v", err)
		return 1
	}
	log.Printf("cv-lifecycle-mail: sent letter %d to %s (ledger untouched)", step, *sendTo)
	return 0
}

// composePreview is Compose, except for the paywall letter, which is about a refused
// attempt and so has no account data to compute: it previews as a fixed sample.
func composePreview(ctx context.Context, content *cvlifecycle.Content, step cvlifecycle.Step, userID int64) (cvlifecycle.Letter, error) {
	if step == cvlifecycle.StepPaywall {
		return cvlifecycle.Letter{Step: step, Paywall: &cvlifecycle.Paywall{
			Feature: "tailor", JobTitle: "Senior Go Engineer", Company: "Example Co", JobSlug: "senior-go-engineer-example-co",
		}}, nil
	}
	return content.Compose(ctx, step, userID)
}
