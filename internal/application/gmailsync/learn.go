package gmailsync

import "context"

// The self-learning ATS-domain cache. It closes the overfitting trap of the
// hardcoded allowlist: instead of a human adding every niche ATS domain seen in
// one inbox, a domain earns its place after its mail is confidently classified as
// job-application mail PromoteThreshold times. BuildQueryFor then unions the promoted
// domains into the sync query, so coverage grows from real classifications.
//
// Wiring (two seams): the write side — mailclassify/maillink calls RecordJobMail
// with the sender when it confidently labels an email as an application — and the
// read side — the sync Worker loads Promoted() and passes it to BuildQueryFor. The
// DB adapter over the learned_ats_domains table (migration 0036) implements
// LearnedDomainStore.

// PromoteThreshold is how many confident job-mail sightings a domain needs before
// it joins the sync allowlist. Above 1 so a single misclassification can't promote
// a newsletter domain.
const PromoteThreshold = 3

// DomainSource is the read side the sync Worker depends on: the promoted domains
// to union into the query. A nil/empty result leaves the query at its hardcoded
// core, so the feature is inert until the store is wired.
type DomainSource interface {
	Promoted(ctx context.Context) ([]string, error)
}

// LearnedDomainStore is the full port: the read side plus Observe, which records
// one confident sighting and returns the domain's running count.
type LearnedDomainStore interface {
	DomainSource
	Observe(ctx context.Context, domain string) (count int, err error)
}

// noLearnedDomains is the default DomainSource: it learns nothing, so a Worker
// built without a store behaves exactly as the hardcoded-only query did.
type noLearnedDomains struct{}

func (noLearnedDomains) Promoted(context.Context) ([]string, error) { return nil, nil }

// freeMailDomains are consumer mailbox providers: a job seeker's peers, personal
// recruiters, and the seeker themselves send from these, so promoting one would
// flood the query. They are never learnable.
var freeMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true,
	"hotmail.com": true, "live.com": true, "yahoo.com": true,
	"icloud.com": true, "me.com": true, "proton.me": true,
	"protonmail.com": true, "aol.com": true, "gmx.com": true,
	"yandex.ru": true, "mail.ru": true,
}

// LearnableDomain extracts the sender domain worth learning from a From header,
// reporting false when it must not be learned: no parseable host, a free-mail
// provider, or a domain already in the hardcoded allowlist (learning it is a
// no-op). The domain is returned lowercased.
func LearnableDomain(from string) (string, bool) {
	host := hostOf(from)
	if host == "" || freeMailDomains[host] {
		return "", false
	}
	if IsATSSender(from) {
		return "", false
	}
	return host, true
}

// RecordJobMail is the write-side entry point the classifier calls once it has
// confidently labeled an email as job-application mail: it learns the sender
// domain (skipping free-mail and already-known domains) so repeated sightings
// promote it. A non-learnable sender is a no-op.
func RecordJobMail(ctx context.Context, store LearnedDomainStore, from string) error {
	domain, ok := LearnableDomain(from)
	if !ok {
		return nil
	}
	_, err := store.Observe(ctx, domain)
	return err
}
