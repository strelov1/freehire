package telegram

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// PendingPost is a claimed telegram_posts row awaiting extraction.
type PendingPost struct {
	Channel  string
	MsgID    int64
	Text     string
	PostedAt time.Time
	Links    []Link
}

// Extractor classifies a post and extracts its vacancies via an LLM. The kind
// steers the prompt (board: expect one vacancy; authored: expect 0..N). The
// result is not trusted — the runner validates before persisting.
type Extractor interface {
	Extract(ctx context.Context, text string, kind Kind) (Extraction, error)
}

// ResolvedJob is a fully-identified vacancy parsed by following a post's outbound link to
// its destination site (e.g. career.habr.com). Unlike an ExtractedJob it carries its own
// source identity, so it is stored under the destination platform, not "telegram".
type ResolvedJob struct {
	Source      string
	ExternalID  string
	URL         string
	Title       string
	Company     string
	Location    string
	Description string
	Remote      bool
	PostedAt    *time.Time
	WorkMode    string
}

// LinkResolver turns a post's outbound links into fully-identified jobs by fetching and
// parsing their destination pages. It returns the jobs from every link a destination
// adapter matched; a non-nil error means matched links existed but all failed (a transient
// failure worth retrying), while no matched link yields (nil, nil) so the caller falls back
// to the LLM. Per-link parse skips and failures are the resolver's concern to log.
type LinkResolver interface {
	Resolve(ctx context.Context, links []Link) ([]ResolvedJob, error)
}

// ExtractStore is the persistence boundary of the extraction worker. Complete
// writes the extracted jobs through the canonical job upsert and marks the post
// extracted in one transaction; Fail counts a failed attempt (dead-lettering at
// the attempt cap is the store's concern).
//
// Both write methods take the built aggregate, not the raw extraction: the domain factory and
// its rejection rule belong to the runner, so a mis-extraction is counted where it happened
// instead of being dropped silently by the adapter and reported as written.
type ExtractStore interface {
	Claim(ctx context.Context, leaseSeconds, batchSize int32) ([]PendingPost, error)
	Complete(ctx context.Context, post PendingPost, jobs []job.Job) error
	// CompleteLinks writes link-resolved jobs (each under its own source identity) and
	// marks the post extracted, the same transactional shape as Complete.
	CompleteLinks(ctx context.Context, post PendingPost, jobs []job.Job) error
	Fail(ctx context.Context, post PendingPost, errMsg string) error
}

// ExtractStats summarizes one extraction run.
type ExtractStats struct {
	Processed int // posts completed (jobs written or none found)
	Jobs      int // vacancies written
	Failed    int // posts whose extraction failed this run
	// Skipped counts vacancies that were produced but not written: dropped by Validate as
	// malformed, or refused by the domain for having no title or identity. Reported rather
	// than folded into Jobs because it is the one signal that says the EXTRACTION is going
	// wrong, as opposed to the posts simply being empty of vacancies.
	//
	// Both sources used to be invisible. Validate drops in place and said nothing; the link
	// path has no Validate at all, so a resolver returning a titleless vacancy had it dropped
	// by the ADAPTER while the runner counted it as written.
	Skipped int
}

// Extraction queue tuning. The lease must outlive the slowest plausible LLM
// call; its expiry doubles as the crash reaper (see the enrichment runner).
const (
	leaseSeconds = 600
	batchSize    = 50
)

// ExtractRunner drains one batch of pending posts: claim, extract, validate,
// persist. A post whose payload is invalid or whose LLM call fails is failed —
// the store retries it once (on a later run, after the lease expires) and then
// dead-letters it; an invalid payload is never persisted.
type ExtractRunner struct {
	Extractor Extractor
	Store     ExtractStore
	Kinds     map[string]Kind // channel → kind, from sources/telegram.yml
	Links     LinkResolver    // optional; resolves outbound job links to full vacancies
}

// Run processes one claimed batch and returns its stats. A post whose links a destination
// adapter resolves is stored from those (deterministic) jobs and the LLM is skipped; any
// other post takes the LLM path.
func (r ExtractRunner) Run(ctx context.Context) (ExtractStats, error) {
	var stats ExtractStats

	posts, err := r.Store.Claim(ctx, leaseSeconds, batchSize)
	if err != nil {
		return stats, err
	}

	for _, post := range posts {
		linkJobs, err := r.resolveLinks(ctx, post)
		if err != nil {
			// Matched links existed but all failed — fail the post so the lease retries it.
			log.Printf("telegram: resolve links %s/%d failed: %v", post.Channel, post.MsgID, err)
			stats.Failed++
			if ferr := r.Store.Fail(ctx, post, err.Error()); ferr != nil {
				return stats, ferr
			}
			continue
		}
		if len(linkJobs) > 0 {
			built, skipped := r.draftLinkJobs(post, linkJobs)
			if err := r.Store.CompleteLinks(ctx, post, built); err != nil {
				return stats, err
			}
			stats.Processed++
			stats.Jobs += len(built)
			stats.Skipped += skipped
			continue
		}

		extraction, err := r.Extractor.Extract(ctx, post.Text, r.kind(post.Channel))
		// Counted before Validate, which drops malformed jobs in place and keeps the rest.
		// Those drops were invisible: a post naming three roles of which two were malformed
		// reported jobs=1 with nothing saying two had been discarded.
		offered := len(extraction.Jobs)
		if err == nil {
			err = extraction.Validate()
		}
		if err != nil {
			log.Printf("telegram: extract %s/%d failed: %v", post.Channel, post.MsgID, err)
			stats.Failed++
			if ferr := r.Store.Fail(ctx, post, err.Error()); ferr != nil {
				return stats, ferr
			}
			continue
		}

		built, refused := r.draftJobs(post, extraction.Jobs)
		if err := r.Store.Complete(ctx, post, built); err != nil {
			return stats, err
		}
		stats.Processed++
		stats.Jobs += len(built)
		stats.Skipped += refused + (offered - len(extraction.Jobs))
	}
	return stats, nil
}

// draftJobs turns a post's extracted vacancies into the aggregate, dropping the ones the
// domain refuses and reporting how many.
//
// It runs here rather than in the adapter because a mis-extraction — a vacancy with no title
// or identity — is a fact about the RUN. While the adapter dropped them, the runner still
// counted them as written, so the one number an operator judges extraction quality by was
// blind to its own failure mode. The crawl path has always built the aggregate in the app
// layer for the same reason (see pipeline.normalizeJob).
func (r ExtractRunner) draftJobs(post PendingPost, jobs []ExtractedJob) (built []job.Job, skipped int) {
	base := post.Channel + "/" + strconv.FormatInt(post.MsgID, 10)
	built = make([]job.Job, 0, len(jobs))
	for i, j := range jobs {
		externalID := base + "/" + strconv.Itoa(i)
		// workMode is empty: a post carries no structured signal, so the location parser
		// decides. The Telegram post's own timestamp is the posting's source posted time.
		posted := post.PostedAt
		one, err := newTelegramJob("telegram", externalID, "https://t.me/"+base,
			j.Title, j.Company, j.Location, j.Remote, TextToHTML(j.Description), "", &posted)
		if err != nil {
			log.Printf("telegram: skipping job %s: %v", externalID, err)
			skipped++
			continue
		}
		built = append(built, one)
	}
	return built, skipped
}

// draftLinkJobs is draftJobs for the vacancies resolved by following a post's links. Their
// identity (source, external id, url) is the destination platform's own, not the post's.
func (r ExtractRunner) draftLinkJobs(post PendingPost, jobs []ResolvedJob) (built []job.Job, skipped int) {
	built = make([]job.Job, 0, len(jobs))
	for _, j := range jobs {
		posted := post.PostedAt
		if j.PostedAt != nil {
			posted = *j.PostedAt
		}
		// A resolved job may state its work mode; job.New gives that precedence over the
		// location parser, and an empty value lets the parser decide.
		one, err := newTelegramJob(j.Source, j.ExternalID, j.URL,
			j.Title, j.Company, j.Location, j.Remote, j.Description, j.WorkMode, &posted)
		if err != nil {
			log.Printf("telegram: skipping link job %s/%s: %v", j.Source, j.ExternalID, err)
			skipped++
			continue
		}
		built = append(built, one)
	}
	return built, skipped
}

// newTelegramJob builds the aggregate through the shared factory, so the dictionary facets
// and slugs are derived exactly as ingest and the moderator path derive them. It returns
// job.ErrInvalidDraft for a vacancy with no title or identity.
func newTelegramJob(source, externalID, url, title, company, loc string, remote bool,
	description, workMode string, postedAt *time.Time) (job.Job, error) {
	return job.New(job.Draft{
		Input: jobderive.Input{
			Source:      source,
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    loc,
			Description: description,
			WorkMode:    workMode,
		},
		URL:    url,
		Remote: remote,
		// It rides the draft rather than being written over the mapped params, so the
		// derived columns fingerprint the posted_at that is actually stored.
		PostedAt: postedAt,
	})
}

// resolveLinks follows a post's outbound links to full vacancies, returning nil when no
// resolver is configured or the post has no links.
func (r ExtractRunner) resolveLinks(ctx context.Context, post PendingPost) ([]ResolvedJob, error) {
	if r.Links == nil || len(post.Links) == 0 {
		return nil, nil
	}
	return r.Links.Resolve(ctx, post.Links)
}

// kind resolves a channel's configured kind, defaulting to board for a post
// whose channel has since left sources/telegram.yml (the safer, single-vacancy prompt).
func (r ExtractRunner) kind(channel string) Kind {
	if k, ok := r.Kinds[channel]; ok {
		return k
	}
	return KindBoard
}
