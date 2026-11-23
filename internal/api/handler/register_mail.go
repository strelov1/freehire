package handler

import (
	"context"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/engage/report"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/platform/db"
)

// apiMail is the SES transport the API's senders share. client and claimMailer are nil when
// SES is unconfigured; links and prefs always exist.
type apiMail struct {
	// links signs the unsubscribe URL every non-essential mail carries; one instance for
	// every API sender keeps footer tokens the same shape.
	links *emailprefs.Links
	// prefs serves the preference routes and answers "has this account turned that group
	// off" for the report notice and referral ping, which have no selection query to gate.
	// One instance, so the footer switch and the sender's check are the same switch.
	prefs *emailprefs.Service
	// client is the transport as its CONCRETE type, because mentorship is composed
	// separately; callers wrap it in an interface only when non-nil (typed-nil trap).
	client *emailnotify.Client
	// claimMailer exists only once an SES client does. employerH is constructed
	// unconditionally (its routes register either way), but its employer.Service needs it.
	claimMailer *emailnotify.AuthMailer
}

// newAPIMail dials SES when AWS_REGION and NOTIFY_EMAIL_FROM are set, and wires the account
// codes and the moderation-decision notice to it.
func newAPIMail(cfg Config, queries *db.Queries, accountsSvc *accounts.Service, reportsH *reportHandlers) apiMail {
	m := apiMail{
		links: emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin),
		prefs: emailprefs.NewService(queries, cfg.JWTSecret),
	}
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("accounts: AWS_REGION/NOTIFY_EMAIL_FROM unset — email verification and password reset are unavailable")
		return m
	}
	ec, err := emailnotify.NewClient(context.Background(), cfg.AWSRegion)
	if err != nil {
		log.Printf("referral: email pinger disabled: %v", err)
		return m
	}
	m.client = ec
	// One AuthMailer carries the account mails (verification, password reset) AND the
	// employer-claim code. Without it the accounts service still registers and
	// authenticates (code-backed flows 503), and employer.Service.Claim reports
	// ErrMailUnavailable before reserving a company slug it could not mail a code for.
	m.claimMailer = emailnotify.NewAuthMailer(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
	accountsSvc.WithCodes(accounts.NewQueriesCodeStore(queries, cfg.Pool), m.claimMailer)
	// Also tells a reporter what a moderator decided; without it each decision reports
	// that nobody was notified.
	reportsH.report.WithNotifier(report.NewMailNotifier(ec, cfg.NotifyEmailFrom, cfg.FrontendOrigin, m.links, m.prefs))
	return m
}

// newDirectApplySurface wires applying on freehire. It needs a CV renderer and a mail
// transport; without either the routes answer 501 rather than record an application that
// is never delivered.
func newDirectApplySurface(cfg Config, queries *db.Queries, mailClient *emailnotify.Client, cvRenderer cv.Renderer, cvPDF ownedCVPDF, resumeStore *resume.Store, employerSvc *employer.Service, employerH *employerHandlers, tracking *jobtracking.Service, answerBank *answerbank.Store) *directApplyHandlers {
	directApplyH := &directApplyHandlers{}
	var applicationMailer *emailnotify.ApplicationMailer
	if mailClient != nil {
		applicationMailer = emailnotify.NewApplicationMailer(mailClient, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
		employerH.messages = directapply.NewApplicationMessages(queries, applicationMailer, cfg.MailboxDomain)
		// The inbox needs no CV renderer — it serves the CV each application kept — but it
		// does need mail: moving an application may email the candidate.
		employerH.inbox = directapply.NewInbox(
			directapply.NewQueriesRepository(queries),
			employerSvc,
			directApplyStatusMailer{mailer: applicationMailer, origin: cfg.FrontendOrigin},
			directapply.NewApplicationMoves(queries, cfg.Pool),
		)
	}
	if cvRenderer != nil && mailClient != nil {
		directApplyH.svc = directapply.New(
			directapply.NewQueriesRepository(queries),
			employerSvc,
			directApplyCVs{pdf: cvPDF, resumes: resumeStore},
			directApplyMailer{mailer: applicationMailer, origin: cfg.FrontendOrigin},
			directApplyTracker{tracking: tracking},
			answerBank,
			cfg.DirectApplyDailyLimit,
		)
	} else {
		log.Print("directapply: no CV renderer or no mail transport — applications on freehire are unavailable")
	}
	return directApplyH
}

// newReferralSurface wires referrals. Notifications reuse the SES transport and the Telegram
// bot when linked. Each channel is wrapped only when configured (typed-nil trap); a referrer
// with no reachable channel still sees the request in-cabinet.
func newReferralSurface(cfg Config, queries *db.Queries, mail apiMail, telegramH *telegramHandlers, cvRenderer cv.Renderer, cvStore *cv.Store, photoStore *headshot.Store, resumeStore *resume.Store) *referralHandlers {
	var referralEmail emailnotify.Sender
	if mail.client != nil {
		referralEmail = mail.client
	}
	var referralTelegram referral.TelegramSender
	if telegramH.telegramBot != nil {
		referralTelegram = telegramH.telegramBot
	}
	referralPinger := referral.NewChannelPinger(referralEmail, cfg.NotifyEmailFrom, referralTelegram, cfg.FrontendOrigin, mail.links, mail.prefs)
	referralCabinetURL := strings.TrimRight(cfg.FrontendOrigin, "/") + "/my/referrals/incoming"
	referralSvc := referral.New(referral.NewQueriesRepository(queries), referralPinger, cfg.Blob,
		referral.Config{CabinetURL: referralCabinetURL})
	return newReferralHandlers(referralSvc, cfg.Blob, cvRenderer, cvStore, photoStore, queries, resumeStore)
}
