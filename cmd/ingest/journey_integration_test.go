//go:build integration

package main

// The data path a posting takes, end to end (#4587): a crawled board, the ingest write,
// the search outbox, the drain into Meilisearch, the projection the API serves, a saved
// search's alert and the email it sends, then a save through the tracking writer and the
// reminder email it produces. Every unit on that path has its own test; this
// one exists for the seams between them, where every failure the catalogue actually had
// lived (a salary stored but never served, a row searchable in Postgres and absent from
// the index). Only external I/O is stubbed: the board is an httptest server replaying a
// recorded Ashby response, and the mailer records instead of calling SES.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
	"github.com/strelov1/freehire/internal/search/serveprobe"
)

func TestJourneyCrawledBoardToDeliveredAlert(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	meili, meiliURL := startJourneyMeili(t)
	if err := meili.EnsureIndex(ctx); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	// 0. Someone subscribed to a saved search before the posting appeared: an alert only
	// carries postings created after its subscription started.
	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified, pro_until_granted)
		VALUES ('journey@example.test', true, now() + interval '1 year') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	// instant is a paid frequency: deliver on this pass rather than at the daily digest time.
	// enabled turns on the saved-job reminders of step 6.
	if _, err := pool.Exec(ctx, `INSERT INTO notification_settings (user_id, digest_frequency, enabled, channels)
		VALUES ($1, 'instant', true, '{email}')`, user); err != nil {
		t.Fatal(err)
	}
	var saved int64
	if err := pool.QueryRow(ctx, `INSERT INTO saved_searches (user_id, name, query) VALUES ($1, 'SRE', 'q=reliability') RETURNING id`, user).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	// A minute back: the index keeps created_at to the second, so a posting crawled in the
	// same second as the subscription would read as older than it.
	if _, err := pool.Exec(ctx, `INSERT INTO subscriptions (user_id, saved_search_id, channel, active, start_at)
		VALUES ($1, $2, 'email', true, now() - interval '1 minute')`, user, saved); err != nil {
		t.Fatal(err)
	}

	// 1. Crawl: the real Ashby adapter reads the recorded board, the real ingest store writes it.
	fixture, err := os.ReadFile("../../internal/ingest/sources/testdata/ashby/board.json")
	if err != nil {
		t.Fatal(err)
	}
	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posting-api/job-board/acme" || r.URL.Query().Get("includeCompensation") != "true" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(fixture)
	}))
	defer board.Close()
	runner := pipeline.Runner{
		Store:    newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()),
		Registry: map[string]sources.Source{"ashby": sources.NewAshby(boardGetter{board.URL})},
	}
	stats, err := runner.Run(ctx, []sources.CompanyEntry{{Provider: "ashby", Board: "acme", Company: "Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	if st := stats["ashby"]; st.Failed != 0 || st.Ingested != 1 {
		t.Fatalf("crawl stats = %+v, want one saved posting", st)
	}

	// 2. Store: the row, and the outbox entry the same write queued for search.
	var id int64
	if err := pool.QueryRow(ctx, `SELECT id FROM jobs WHERE source = 'ashby'`).Scan(&id); err != nil {
		t.Fatalf("crawled row: %v", err)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_outbox WHERE job_id = $1`, id).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("search_outbox entries = %d (%v), want 1", queued, err)
	}
	row := readJob(t, q, id)
	if !row.SalaryMinSource.Valid || row.SalaryMinSource.Int32 != 90000 || row.SalaryCurrencySource != "EUR" {
		t.Fatalf("stored salary = %v %q, want the board's 90000 EUR", row.SalaryMinSource, row.SalaryCurrencySource)
	}
	withheld := func(j db.Job) search.WithholdReason {
		return search.WithheldBy(j, time.Now(), search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow)))
	}

	// Before the drain the posting is in Postgres and not in the index: the window the
	// serve probe reports as missing. The rest of the test proves the drain closes it.
	docs, err := meili.JobViewsByID(ctx, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if rep := serveprobe.Check([]db.Job{row}, docs, withheld); rep.Unindexed[serveprobe.Missing] != 1 {
		t.Fatalf("undrained posting: unindexed = %v, want it missing from the index", rep.Unindexed)
	}

	// 3. Drain the outbox into Meilisearch.
	drained, err := searchdrain.Runner{Store: journeyDrainStore{q}, Indexer: journeyIndexer{meili}}.
		Run(ctx, searchdrain.RunOptions{BatchSize: 10, LeaseSeconds: 60, MaxAttempts: 3})
	if err != nil || drained.Indexed != 1 || drained.Failed != 0 {
		t.Fatalf("drain = %+v (%v), want one indexed", drained, err)
	}

	// 4. Served: every field the row stores reaches both the API projection and the index
	// document (the serve-probe rule table), and the two agree field for field.
	row = readJob(t, q, id)
	docs, err = meili.JobViewsByID(ctx, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	rep := serveprobe.Check([]db.Job{row}, docs, withheld)
	if len(rep.Unindexed) != 0 || rep.Unreadable != 0 {
		t.Fatalf("drained posting: unindexed = %v unreadable = %d", rep.Unindexed, rep.Unreadable)
	}
	for _, field := range []string{"salary", "countries", "work_mode", "employment_type"} {
		if rep.Counts[serveprobe.LayerProjection][field].Stored != 1 {
			t.Errorf("the fixture should store %s; the probe would compare nothing", field)
		}
	}
	for _, layer := range serveprobe.Layers {
		for field, c := range rep.Counts[layer] {
			if c.Gaps != 0 {
				t.Errorf("the row stores %s but the %s layer does not serve it", field, layer)
			}
		}
	}
	projected, err := jobview.FromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	compareServed(t, projected, docs[id], rawDocument(t, meiliURL, id))

	// 5. Alert: the real notify engine matches the saved search against the index and
	// delivers through the real email renderer.
	mailer := &journeyMailer{}
	const origin = "https://freehire.test"
	links := emailprefs.NewLinks("journey-test-secret-at-least-32-bytes", origin)
	router := notify.Router{notify.ChannelEmail: emailnotify.NewNotifier(mailer, "alerts@freehire.test", origin, links)}
	notified, err := notify.New(q, meili, router, notify.DefaultConfig()).Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if notified.Matched != 1 || notified.Delivered != 1 || len(mailer.sent) != 1 {
		t.Fatalf("notify = %+v, mails = %d; want the posting matched and one email", notified, len(mailer.sent))
	}
	mail := mailer.sent[0]
	if mail.To != "journey@example.test" {
		t.Errorf("mail to %q", mail.To)
	}
	for _, want := range []string{row.Title, "Acme", "€90K—€120K / year", origin + "/jobs/" + row.PublicSlug} {
		if !strings.Contains(mail.Text, want) || !strings.Contains(mail.HTML, want) {
			t.Errorf("alert email lacks %q:\n%s", want, mail.Text)
		}
	}
	var recorded int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_notifications WHERE user_id = $1`, user).Scan(&recorded); err != nil || recorded != 1 {
		t.Errorf("in-app notifications = %d (%v), want the delivered digest recorded", recorded, err)
	}

	// 6. Save: the real tracking writer schedules the come-back reminder, and the reminder
	// engine delivers it by email once due.
	tracking := jobtracking.New(jobtracking.NewQueriesRepository(q, pool), jobtracking.WithReminders(reminder.New(reminder.NewQueriesRepository(q))))
	if _, err := tracking.SaveJob(ctx, user, row.PublicSlug); err != nil {
		t.Fatal(err)
	}
	// It fires days out; bring it due.
	if tag, err := pool.Exec(ctx, `UPDATE job_reminders SET fire_at = now() - interval '1 minute' WHERE user_id = $1 AND job_id = $2`, user, id); err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("saved job scheduled %d reminders (%v), want 1", tag.RowsAffected(), err)
	}
	mailer.sent = nil
	reminders := reminder.Router{notify.ChannelEmail: reminder.NewEmailNotifier(mailer, "alerts@freehire.test", origin, links)}
	if _, err := reminder.NewRunner(q, reminders, reminder.DefaultConfig()).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(mailer.sent) != 1 || !strings.Contains(mailer.sent[0].Text, row.Title) || !strings.Contains(mailer.sent[0].Text, origin+"/jobs/"+row.PublicSlug) {
		t.Fatalf("reminder emails = %+v, want one linking the saved posting", mailer.sent)
	}
}

// compareServed fails on any JSON field the projection and the index document disagree
// on, a key one serves and the other omits included. Decoding the document zero-fills a
// field it lacks, so presence is checked on the raw stored document.
func compareServed(t *testing.T, projected, indexed jobview.Job, raw map[string]json.RawMessage) {
	t.Helper()
	p, x := jsonFields(t, projected), jsonFields(t, indexed)
	for _, k := range slices.Sorted(maps.Keys(p)) {
		if _, ok := raw[k]; !ok {
			t.Errorf("%s: projection serves %s, the stored index document lacks it", k, p[k])
		}
	}
	keys := slices.Sorted(maps.Keys(p))
	for k := range x {
		if _, ok := p[k]; !ok {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		if string(p[k]) != string(x[k]) {
			t.Errorf("%s: projection serves %s, the index document %s", k, p[k], x[k])
		}
	}
}

func jsonFields(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func readJob(t *testing.T, q *db.Queries, id int64) db.Job {
	t.Helper()
	rows, err := q.GetJobsByIDs(context.Background(), []int64{id})
	if err != nil || len(rows) != 1 {
		t.Fatalf("read job %d: %v (%d rows)", id, err, len(rows))
	}
	return rows[0]
}

// journeyMeiliKey is the throwaway container's master key.
const journeyMeiliKey = "test-master-key"

// rawDocument reads job id's document from the jobs index as stored, without decoding it.
func rawDocument(t *testing.T, endpoint string, id int64) map[string]json.RawMessage {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("%s/indexes/jobs/documents/%d", endpoint, id), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+journeyMeiliKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]json.RawMessage
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&doc) != nil {
		t.Fatalf("raw document %d: status %d", id, resp.StatusCode)
	}
	return doc
}

// boardGetter is the adapter's HTTP client pointed at the recorded board: same path and
// query, another host.
type boardGetter struct{ base string }

func (g boardGetter) GetJSON(ctx context.Context, raw string, v any) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+u.RequestURI(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("board answered %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// journeyDrainStore and journeyIndexer are cmd/search-drain's adapters cut to the outbox
// queries and the document builder, since a main package cannot import another.
// shortcut: the indexer skips the reality signal, cluster geography and apply-question
// attachments cmd/search-drain adds; move its adapters into internal/search/searchdrain
// if a seam there needs this test.
type journeyDrainStore struct{ q *db.Queries }

func (s journeyDrainStore) Claim(ctx context.Context, batch, lease int) ([]searchdrain.Claimed, error) {
	rows, err := s.q.ClaimSearchOutboxBatch(ctx, db.ClaimSearchOutboxBatchParams{LeaseSeconds: int32(lease), BatchSize: int32(batch)}) //nolint:gosec // the test's own small RunOptions
	out := make([]searchdrain.Claimed, len(rows))
	for i, r := range rows {
		out[i] = searchdrain.Claimed{OutboxID: r.ID, JobID: r.JobID}
	}
	return out, err
}

func (s journeyDrainStore) Jobs(ctx context.Context, ids []int64) ([]db.Job, error) {
	return s.q.GetJobsByIDs(ctx, ids)
}

func (s journeyDrainStore) Complete(ctx context.Context, entries []searchdrain.Claimed) error {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.OutboxID
	}
	return s.q.DeleteSearchOutboxEntries(ctx, ids)
}

func (s journeyDrainStore) Fail(ctx context.Context, id int64, msg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordSearchOutboxFailure(ctx, db.RecordSearchOutboxFailureParams{LastError: msg, MaxAttempts: int32(maxAttempts), ID: id}) //nolint:gosec // the test's own small RunOptions
	return row.FailedAt.Valid, err
}

func (s journeyDrainStore) Reap(ctx context.Context, maxRows int) (int64, error) {
	return s.q.DeleteIneligibleSearchOutbox(ctx, int32(maxRows)) //nolint:gosec // the test's own small RunOptions
}

type journeyIndexer struct{ client *search.Client }

func (ix journeyIndexer) IndexBatch(ctx context.Context, jobs []db.Job) error {
	sourceType := sources.SourceTypes(sources.Taxonomy())
	docs := make([]search.JobDocument, 0, len(jobs))
	for _, j := range jobs {
		doc, err := search.FromJob(j, sourceType(j.Source))
		if err != nil {
			return err
		}
		docs = append(docs, doc)
	}
	return ix.client.IndexJobs(ctx, docs)
}

// journeyMailer records the alert emails instead of sending them through SES.
type journeyMailer struct{ sent []emailnotify.Message }

func (m *journeyMailer) Send(_ context.Context, msg emailnotify.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

func startJourneyMeili(t *testing.T) (*search.Client, string) {
	t.Helper()
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "getmeili/meilisearch:v1.49.0", // the version prod runs (docker-compose.yml)
			ExposedPorts: []string{"7700/tcp"},
			Env:          map[string]string{"MEILI_MASTER_KEY": journeyMeiliKey, "MEILI_ENV": "development"},
			WaitingFor:   wait.ForHTTP("/health").WithPort("7700/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start meilisearch: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	endpoint, err := c.PortEndpoint(ctx, "7700/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	return search.NewClient(endpoint, journeyMeiliKey), endpoint
}
