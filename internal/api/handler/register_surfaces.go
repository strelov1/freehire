package handler

import (
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/bookinglink"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The surfaces Register composes in more than one step, each built from the shared
// services Register hands it.

// newConfiguredAuthHandlers builds the auth surface with every sign-in path cfg enables.
func newConfiguredAuthHandlers(cfg Config, queries *db.Queries, issuer *auth.Issuer) *authHandlers {
	authH := newAuthHandlers(queries, cfg.Pool, issuer, cfg.CookieSecure, cfg.CookieDomains, cfg.OAuthRegistry, cfg.FrontendOrigin, cfg.ExtensionRedirectAllowlist,
		cfg.ServedHosts)
	authH.authV2Enabled = cfg.AuthV2Enabled
	authH.mobileCallbacks = cfg.MobileAuthCallbacks
	authH.mobileAuth = mobileauth.NewStore(cfg.Pool)
	authH.recentAuth = recentauth.NewStore(cfg.Pool, cfg.RecentAuthTTL)
	authH.appleNative = cfg.AppleNative
	authH.appleGrantKeys = cfg.AppleGrantKeys
	if needsExplicitServedHosts(cfg.ServedHosts, cfg.CookieDomains) {
		log.Printf("oauth: COOKIE_DOMAIN is set (%v) but SERVED_HOSTS is not — "+
			"the redirect origin falls back to %s for every other host, which breaks "+
			"sign-in on them (state mismatch). List every served host in SERVED_HOSTS.",
			cfg.CookieDomains, cfg.FrontendOrigin)
	}
	return authH
}

// newBuddySurfaces builds the messaging core with the buddy persona registered, and the
// buddy surfaces over it.
func newBuddySurfaces(cfg Config, queries *db.Queries, mailClient *emailnotify.Client) (*conversationHandlers, *buddyHandlers) {
	// Conversations belong to the messaging core; each persona a participant may speak as is
	// registered here.
	buddyRepo := buddy.NewQueriesRepository(queries, cfg.Pool)
	messagingSvc := messaging.New(messaging.NewQueriesRepository(queries), buddy.NewPersona(buddyRepo))
	buddySvc := buddy.New(buddyRepo, messagingSvc)
	// Buddy mock interviews. Without a mail transport notices still reach the notification
	// center. Must be a properly nil interface, never a typed-nil *Client.
	var mockMail emailnotify.Sender
	if mailClient != nil && cfg.NotifyEmailFrom != "" {
		mockMail = mailClient
	}
	buddyMocks := buddy.NewMockService(buddySvc, buddyRepo,
		buddy.NewMockMailNotifier(queries, mockMail, cfg.NotifyEmailFrom, cfg.FrontendOrigin))
	conversationH := newConversationHandlers(messagingSvc, buddyMocks)
	buddyH := newBuddyHandlers(buddySvc, buddyMocks)

	return conversationH, buddyH
}

// newMentorshipSurfaces builds the mentorship marketplace and its profile-form prefill.
func newMentorshipSurfaces(cfg Config, queries *db.Queries, mailClient *emailnotify.Client, photoStore *headshot.Store, resumeStore *resume.Store, profileSvc *userprofile.Service, bank *experience.Store) (*mentorshipHandlers, *mentorSuggestionsHandlers) {
	// The mentorship marketplace: referral's opposite, where the insider is named and chosen.
	// A nil notifier means no mail transport: the feature works and nobody is told.
	var mentorshipNotifier mentorship.Notifier
	if mailClient != nil && cfg.NotifyEmailFrom != "" {
		mentorshipNotifier = mentorship.NewMailNotifier(mailClient, cfg.NotifyEmailFrom,
			strings.TrimRight(cfg.FrontendOrigin, "/")+"/my/sessions")
	}
	// A connected mentor's calendar.events grant, reusing the candidate calendar sync's
	// GmailConnector/GmailCipher pair. Nil when either is unset, so bookings get the
	// static-link fallback.
	mentorshipRepo := mentorship.NewQueriesRepository(queries, cfg.Pool)
	var mentorshipCalendar mentorship.CalendarLinker
	if cfg.GmailConnector != nil && cfg.GmailCipher != nil {
		mentorshipCalendar = mentorship.NewGoogleCalendarLinker(mentorshipRepo, cfg.GmailConnector, cfg.GmailCipher)
	}
	mentorshipH := newMentorshipHandlers(mentorship.New(
		mentorshipRepo,
		mentorship.Config{Notifier: mentorshipNotifier, Cache: cfg.Cache, CalendarLinker: mentorshipCalendar},
	), photoStore)
	// The mentor-profile create form's prefill, composed from the CV, user profile, account and
	// experience bank services plus the company catalog, never from mentorship itself.
	//
	// The account read is the bare repository, not a second accounts.Service: it only needs
	// UserByID, and newAuthHandlers already builds the one Service (hoist, don't duplicate).
	mentorSuggestionsH := newMentorSuggestionsHandlers(
		resumeStore, profileSvc, accounts.NewQueriesRepository(queries, cfg.Pool), bank, queries,
	)
	return mentorshipH, mentorSuggestionsH
}

// newEmployerSurfaces builds the employer service, with claim and team-invite mail when a
// transport exists, and the employer surface over it.
func newEmployerSurfaces(cfg Config, queries *db.Queries, accountsSvc *accounts.Service, employerClaimMailer *emailnotify.AuthMailer, mailClient *emailnotify.Client, moderationSvc *moderation.Service) (*employer.Service, *employerHandlers) {
	// employer.Service reuses the auth surface's accounts service as its codeIssuer (the
	// pointer WithCodes configured in Register). employerMailer must be a NIL INTERFACE when employerClaimMailer is
	// nil: Service.Claim tests the interface, so a typed nil would pass its check.
	var employerMailer employer.ClaimMailer
	if employerClaimMailer != nil {
		employerMailer = employerClaimMailer
	}
	employerRepo := employer.NewQueriesRepository(queries, cfg.Pool)
	employerSvc := employer.New(employerRepo, accountsSvc, employerMailer, employerRepo, moderationSvc)
	// A properly nil InviteMailer without mail, for the typed-nil reason given above.
	var teamInvites employer.InviteMailer
	if mailClient != nil {
		teamInvites = teamInviteMailer{emailnotify.NewTeamInviteMailer(mailClient, cfg.NotifyEmailFrom, cfg.FrontendOrigin)}
	}
	employerSvc.WithTeam(employerRepo, teamInvites, strings.TrimRight(cfg.FrontendOrigin, "/")+"/my/employer/join")
	return employerSvc, newEmployerHandlers(queries, employerSvc)
}

// wireOptional hands the agent the services that may be unconfigured. Each is assigned
// only when present: a nil value in an interface field is non-nil, so "no voice mode" or
// "no support mail" would become a panic on first use.
func (h *assistantHandlers) wireOptional(cfg Config, store *assistant.Store, answerBank *answerbank.Store, reportsH *reportHandlers, mailClient *emailnotify.Client) {
	if cfg.InterviewAudio != nil {
		h.interviewAudio = cfg.InterviewAudio
	}
	if cfg.Realtime != nil {
		h.realtime = cfg.Realtime
	}
	if p := newInngestEventPublisher(cfg.InngestEventAPIURL, cfg.InngestEventKey); p != nil {
		h.events = p
	}
	h.answerBank = answerBank
	h.support.pool = cfg.Pool
	if cfg.SupportLLM != nil {
		supportLLM := cfg.SupportLLM.WithTimeout(assistantLLMTimeout)
		h.support.llm = supportLLM
		h.support.runner = assistant.NewRunner(supportLLM, store, assistant.RunnerConfig{MaxSteps: supportMaxSteps})
	}
	h.reports = reportsH
	if mailClient != nil && cfg.SupportReplyTo != "" {
		h.support.mail = supportMail{sender: mailClient, from: emailnotify.From("freehire", cfg.NotifyEmailFrom), to: cfg.SupportReplyTo}
	}
}

func newConfiguredInboxHandlers(cfg Config, q *db.Queries, plans *plan.Store) *inboxHandlers {
	h := newInboxHandlers(q, cfg.Pool, cfg.GmailConnector, cfg.GmailCipher, cfg.FrontendOrigin, cfg.CookieSecure, cfg.MailboxDomain)
	h.scheduling = schedulingAccess{
		enabled:     cfg.InterviewSchedulingEnabled && cfg.LLM != nil,
		pilotUserID: cfg.InterviewSchedulingPilotUserID,
		plans:       plans,
	}
	return h
}

func newBookingLinkSurface(cfg Config, q *db.Queries, plans *plan.Store) *bookingLinkHandlers {
	var calendar bookinglink.Calendar
	if cfg.GmailConnector != nil && cfg.GmailCipher != nil {
		calendar = bookinglink.NewGoogleCalendar(q, cfg.GmailConnector, cfg.GmailCipher)
	}
	// A database dump alone cannot rebuild a link's URL.
	service := bookinglink.New(bookinglink.NewQueriesRepository(q, cfg.Pool), plans, []byte(cfg.JWTSecret), calendar)
	return newBookingLinkHandlers(service, cfg.FrontendOrigin)
}
