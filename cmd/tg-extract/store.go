package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/classify"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobfacts"
	"github.com/strelov1/freehire/internal/lang"
	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/skilltag"
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
		geo := location.Parse(j.Location)
		class := classify.Parse(j.Title)
		descHTML := telegram.TextToHTML(j.Description)
		saved, err := qtx.UpsertJob(ctx, db.UpsertJobParams{
			Source:      "telegram",
			ExternalID:  externalID,
			URL:         "https://t.me/" + base,
			Title:       j.Title,
			Company:     j.Company,
			CompanySlug: normalize.Slug(j.Company),
			PublicSlug:  normalize.JobSlug(j.Title, j.Company, "telegram", externalID),
			Location:    j.Location,
			Remote:      j.Remote,
			Description: descHTML,
			PostedAt:    pgtype.Timestamptz{Time: post.PostedAt, Valid: true},
			Countries:   geo.Countries,
			Regions:     geo.Regions,
			Cities:      geo.Cities,
			WorkMode:    geo.WorkMode,
			Skills:      skilltag.Parse(descHTML),
			Seniority:   class.Seniority,
			Category:    class.Category,

			PostingLanguage:    lang.Detect(descHTML),
			EmploymentType:     jobfacts.EmploymentType(j.Title, descHTML),
			EducationLevel:     jobfacts.EducationLevel(descHTML),
			EnglishLevel:       jobfacts.EnglishLevel(descHTML),
			ExperienceYearsMin: toInt4(jobfacts.ExperienceYearsMin(descHTML)),
		})
		if err != nil {
			return fmt.Errorf("upsert job %s: %w", externalID, err)
		}
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion: int32(enrich.Version),
			JobID:         saved.Job.ID,
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
		geo := location.Parse(j.Location)
		class := classify.Parse(j.Title)
		workMode := j.WorkMode
		if workMode == "" {
			workMode = geo.WorkMode
		}
		postedAt := pgtype.Timestamptz{Time: post.PostedAt, Valid: true}
		if j.PostedAt != nil {
			postedAt = pgtype.Timestamptz{Time: *j.PostedAt, Valid: true}
		}
		saved, err := qtx.UpsertJob(ctx, db.UpsertJobParams{
			Source:      j.Source,
			ExternalID:  j.ExternalID,
			URL:         j.URL,
			Title:       j.Title,
			Company:     j.Company,
			CompanySlug: normalize.Slug(j.Company),
			PublicSlug:  normalize.JobSlug(j.Title, j.Company, j.Source, j.ExternalID),
			Location:    j.Location,
			Remote:      j.Remote,
			Description: j.Description,
			PostedAt:    postedAt,
			Countries:   geo.Countries,
			Regions:     geo.Regions,
			Cities:      geo.Cities,
			WorkMode:    workMode,
			Skills:      skilltag.Parse(j.Description),
			Seniority:   class.Seniority,
			Category:    class.Category,

			PostingLanguage:    lang.Detect(j.Description),
			EmploymentType:     jobfacts.EmploymentType(j.Title, j.Description),
			EducationLevel:     jobfacts.EducationLevel(j.Description),
			EnglishLevel:       jobfacts.EnglishLevel(j.Description),
			ExperienceYearsMin: toInt4(jobfacts.ExperienceYearsMin(j.Description)),
		})
		if err != nil {
			return fmt.Errorf("upsert job %s/%s: %w", j.Source, j.ExternalID, err)
		}
		if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
			TargetVersion: int32(enrich.Version),
			JobID:         saved.Job.ID,
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
