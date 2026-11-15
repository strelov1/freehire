package handler

import (
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/identity/auth/recentauth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
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
	// Conversations are the messaging core's; each persona a participant may speak as is
	// registered here.
	buddyRepo := buddy.NewQueriesRepository(queries, cfg.Pool)
	messagingSvc := messaging.New(messaging.NewQueriesRepository(queries), buddy.NewPersona(buddyRepo))
	buddySvc := buddy.New(buddyRepo, messagingSvc)
	// Buddy mock interviews. Without a mail transport the notices still reach the
	// notification center. A properly nil interface, never a typed-nil *Client.
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
	// The mentorship marketplace: referral's opposite number, where the insider is named
	// and chosen rather than anonymous. A nil notifier is a deployment with no mail
	// transport — the feature works and nobody is told, which is how it ships before SES
	// is wired for it and what rolling it back looks like.
	var mentorshipNotifier mentorship.Notifier
	if mailClient != nil && cfg.NotifyEmailFrom != "" {
		mentorshipNotifier = mentorship.NewMailNotifier(mailClient, cfg.NotifyEmailFrom,
			strings.TrimRight(cfg.FrontendOrigin, "/")+"/my/sessions")
	}
	// A connected mentor's calendar.events grant, mirroring the inbox recall gate in Register: the
	// candidate calendar sync's own GmailConnector/GmailCipher pair, reused rather than a
	// second copy of either. Nil when either is unset, so a deployment with no Google
	// client configured just gets every booking's static-link fallback — this feature's
	// entire rollback path.
	mentorshipRepo := mentorship.NewQueriesRepository(queries, cfg.Pool)
	var mentorshipCalendar mentorship.CalendarLinker
	if cfg.GmailConnector != nil && cfg.GmailCipher != nil {
		mentorshipCalendar = mentorship.NewGoogleCalendarLinker(mentorshipRepo, cfg.GmailConnector, cfg.GmailCipher)
	}
	mentorshipH := newMentorshipHandlers(mentorship.New(
		mentorshipRepo,
		mentorship.Config{Notifier: mentorshipNotifier, Cache: cfg.Cache, CalendarLinker: mentorshipCalendar},
	), photoStore)
	// The mentor-profile create form's prefill. Composed from four unrelated blocks'
	// own services (résumé, user profile, account, experience bank) plus the company
	// catalog — never from mentorship itself, see the mentor-profile-prefill spec.
	//
	// The account read is the bare repository, not a second accounts.Service: the
	// only thing this handler needs is UserByID, which accounts.Repository already
	// exposes, and newAuthHandlers already builds the one Service this process
	// needs. A constructor that builds a shared service is the bug this package's
	// AGENTS.md calls out by name — hoist, don't duplicate.
	mentorSuggestionsH := newMentorSuggestionsHandlers(
		resumeStore, profileSvc, accounts.NewQueriesRepository(queries, cfg.Pool), bank, queries,
	)
	return mentorshipH, mentorSuggestionsH
}
