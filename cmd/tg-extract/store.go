package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/telegram"
)

// toInt4 maps an optional int (experience_years_min) to the pgtype the generated
// params expect; a nil pointer becomes SQL NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}

// buildParams constructs the UpsertJob params for one Telegram-sourced job through
// the Job aggregate factory, so the dictionary facets and slugs are derived exactly
// as ingest and the moderator path derive them (job.New wraps jobderive) — no more
// inline derivation that could drift from the shared dictionaries. workMode carries
// a structured signal (link-resolved jobs may state it); "" lets the parser decide.
// It returns job.ErrInvalidDraft for an extracted job with no title/identity.
func buildParams(source, externalID, url, title, company, loc string, remote bool, description, workMode string, postedAt pgtype.Timestamptz) (db.UpsertJobParams, error) {
	j, err := job.New(job.Draft{
		Source:      source,
		ExternalID:  externalID,
		URL:         url,
		Title:       title,
		Company:     company,
		Location:    loc,
		Remote:      remote,
		Description: description,
		WorkMode:    workMode,
	})
	if err != nil {
		return db.UpsertJobParams{}, err
	}
	f := j.Fields()
	params := db.UpsertJobParams{
		Source:      f.Source,
		ExternalID:  f.ExternalID,
		URL:         f.URL,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		PublicSlug:  f.PublicSlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    postedAt,
		Countries:   f.Countries,
		Regions:     f.Regions,
		Cities:      f.Cities,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: toInt4(f.ExperienceYearsMin),
	}
	params.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(params), Valid: true}
	return params, nil
}

// maxAttempts is the retry budget per post: the first failure leaves the post
// retryable (after its lease expires), the second dead-letters it.
const maxAttempts = 2

// extractStore adapts the generated queries + pool to telegram.ExtractStore.
// Complete writes every extracted job through the canonical UpsertJob (which
// upserts the company and gates on the dedup key) plus the enrichment enqueue,
// and marks the post extracted — all in one transaction, so a crash never
// half-persists a post.
type extractStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newExtractStore(pool *pgxpool.Pool) *extractStore {
	return &extractStore{pool: pool, q: db.New(pool)}
}

func (s *extractStore) Claim(ctx context.Context, leaseSeconds, batchSize int32) ([]telegram.PendingPost, error) {
	rows, err := s.q.ClaimTelegramPosts(ctx, db.ClaimTelegramPostsParams{
		LeaseSeconds: leaseSeconds,
		BatchSize:    batchSize,
	})
	if err != nil {
		return nil, err
	}
	posts := make([]telegram.PendingPost, len(rows))
	for i, r := range rows {
		posts[i] = telegram.PendingPost{
			Channel:  r.Channel,
			MsgID:    r.MsgID,
			Text:     r.Text,
			PostedAt: r.PostedAt.Time,
			Links:    decodeLinks(r.Links),
		}
	}
	return posts, nil
}

// decodeLinks unmarshals the stored links JSON, tolerating an empty/legacy NULL column.
func decodeLinks(b []byte) []telegram.Link {
	if len(b) == 0 {
		return nil
	}
	var links []telegram.Link
	if err := json.Unmarshal(b, &links); err != nil {
		return nil
	}
	return links
}

func (s *extractStore) Complete(ctx context.Context, post telegram.PendingPost, jobs []telegram.ExtractedJob) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	base := post.Channel + "/" + strconv.FormatInt(post.MsgID, 10)
	for i, j := range jobs {
		externalID := base + "/" + strconv.Itoa(i)
		descHTML := telegram.TextToHTML(j.Description)
		params, err := buildParams("telegram", externalID, "https://t.me/"+base,
			j.Title, j.Company, j.Location, j.Remote, descHTML, "",
			pgtype.Timestamptz{Time: post.PostedAt, Valid: true})
		if err != nil {
			// An extracted job with no title/identity is junk (a mis-extraction); skip it
			// rather than persisting it or failing the whole post.
			log.Printf("tg-extract: skipping job %s: %v", externalID, err)
			continue
		}
		saved, err := qtx.UpsertJob(ctx, params)
		if err != nil {
			return fmt.Errorf("upsert job %s: %w", externalID, err)
		}
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion:     int32(enrich.Version),
			JobID:             saved.Job.ID,
			ExcludeCategories: enrich.NonTechCategories,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment %s: %w", externalID, err)
		}
	}

	if err := qtx.MarkTelegramPostExtracted(ctx, db.MarkTelegramPostExtractedParams{
		Channel: post.Channel,
		MsgID:   post.MsgID,
	}); err != nil {
		return fmt.Errorf("mark extracted %s: %w", base, err)
	}
	return tx.Commit(ctx)
}

// CompleteLinks writes link-resolved jobs — each under the destination platform's own
// source identity — through the canonical UpsertJob + enrichment enqueue, and marks the
// post extracted, all in one transaction. Same shape as Complete; the identity (source,
// external_id, url) comes from the resolved job rather than the Telegram post.
func (s *extractStore) CompleteLinks(
	ctx context.Context,
	post telegram.PendingPost,
	jobs []telegram.ResolvedJob,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	for _, j := range jobs {
		postedAt := pgtype.Timestamptz{Time: post.PostedAt, Valid: true}
		if j.PostedAt != nil {
			postedAt = pgtype.Timestamptz{Time: *j.PostedAt, Valid: true}
		}
		// The resolved job may carry a structured work-mode; job.New gives it precedence
		// over the location parser (an empty value lets the parser decide), matching the
		// prior j.WorkMode || geo.WorkMode fallback.
		params, err := buildParams(j.Source, j.ExternalID, j.URL,
			j.Title, j.Company, j.Location, j.Remote, j.Description, j.WorkMode, postedAt)
		if err != nil {
			log.Printf("tg-extract: skipping link job %s/%s: %v", j.Source, j.ExternalID, err)
			continue
		}
		saved, err := qtx.UpsertJob(ctx, params)
		if err != nil {
			return fmt.Errorf("upsert job %s/%s: %w", j.Source, j.ExternalID, err)
		}
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion:     int32(enrich.Version),
			JobID:             saved.Job.ID,
			ExcludeCategories: enrich.NonTechCategories,
		}); err != nil {
			return fmt.Errorf("enqueue enrichment %s/%s: %w", j.Source, j.ExternalID, err)
		}
	}

	if err := qtx.MarkTelegramPostExtracted(ctx, db.MarkTelegramPostExtractedParams{
		Channel: post.Channel,
		MsgID:   post.MsgID,
	}); err != nil {
		return fmt.Errorf("mark extracted %s/%d: %w", post.Channel, post.MsgID, err)
	}
	return tx.Commit(ctx)
}

func (s *extractStore) Fail(ctx context.Context, post telegram.PendingPost, errMsg string) error {
	_, err := s.q.RecordTelegramPostFailure(ctx, db.RecordTelegramPostFailureParams{
		LastError:   errMsg,
		MaxAttempts: maxAttempts,
		Channel:     post.Channel,
		MsgID:       post.MsgID,
	})
	return err
}

var _ telegram.ExtractStore = (*extractStore)(nil)
