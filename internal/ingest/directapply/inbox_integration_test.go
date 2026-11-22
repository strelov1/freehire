//go:build integration

// Integration tests for the employer's inbox over the real direct_applications rows: what
// an employer may see and move, and the CV copy each attempt keeps.
// Run with: go test -tags=integration ./internal/ingest/directapply/
package directapply_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type inboxFixture struct {
	pool      *pgxpool.Pool
	repo      *directapply.QueriesRepository
	employer  int64
	other     int64
	vacancy   int64
	otherVac  int64
	candidate int64
}

func newInboxFixture(t *testing.T) inboxFixture {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Pool(t)
	f := inboxFixture{pool: pool, repo: directapply.NewQueriesRepository(db.New(pool))}
	user := func(email string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		return id
	}
	f.employer, f.other, f.candidate = user("hr@acme.test"), user("hr@globex.test"), user("c@example.test")
	// A vacancy belongs to the company team of whoever published it (employer_team), so each
	// employer holds an account.
	for _, a := range []struct {
		id   int64
		slug string
	}{{f.employer, "acme"}, {f.other, "globex"}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status)
			VALUES ($1, $2, $2, $2 || '@example.test', 'active')`, a.id, a.slug); err != nil {
			t.Fatalf("seed account %s: %v", a.slug, err)
		}
	}
	job := func(slug string, by int64) int64 {
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
			VALUES ('employer', 'freehire:' || $1, '', 'Platform Engineer', 'Acme', 'acme', $1, $2) RETURNING id`,
			slug, by).Scan(&id); err != nil {
			t.Fatalf("seed job %s: %v", slug, err)
		}
		return id
	}
	f.vacancy, f.otherVac = job("acme-platform", f.employer), job("globex-sre", f.other)
	return f
}

// apply records and delivers an application with the given CV content.
func (f inboxFixture) apply(t *testing.T, jobID, userID int64, cv string, deliver bool) int64 {
	t.Helper()
	ctx := context.Background()
	id, recorded, err := f.repo.Record(ctx, directapply.Record{
		JobID: jobID, UserID: userID, CV: directapply.CVChoice{Resume: true}, Note: "hi",
		Answers: []directapply.Answer{{ID: "q1", Label: "Stack", Type: "text", Answer: "Go"}},
		CVFile:  directapply.Attachment{Filename: "cv.txt", ContentType: "text/plain; charset=utf-8", Content: []byte(cv)},
	}, 30)
	if err != nil || !recorded {
		t.Fatalf("Record = %d, %v, %v", id, recorded, err)
	}
	if deliver {
		if err := f.repo.MarkSent(ctx, id); err != nil {
			t.Fatalf("MarkSent: %v", err)
		}
	}
	return id
}

func TestInboxRepository_ShowsOnlyTheEmployersDeliveredApplications(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	mine := f.apply(t, f.vacancy, f.candidate, "first", true)
	f.apply(t, f.otherVac, f.candidate, "elsewhere", true)

	apps, err := f.repo.List(ctx, f.employer, directapply.InboxFilter{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != mine {
		t.Fatalf("apps = %+v, want only the application to the employer's own vacancy", apps)
	}
	a := apps[0]
	if a.JobSlug != "acme-platform" || a.CandidateEmail != "c@example.test" || a.Stage != directapply.StageNew ||
		!a.HasCV || a.CVKind != "resume" || len(a.Answers) != 1 || a.Answers[0].Answer != "Go" || a.StageChangedAt != nil {
		t.Errorf("application = %+v", a)
	}
	if _, err := f.repo.Get(ctx, f.other, mine); !errors.Is(err, directapply.ErrNotFound) {
		t.Errorf("another employer's Get err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.CV(ctx, f.other, mine); !errors.Is(err, directapply.ErrNotFound) {
		t.Errorf("another employer's CV err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.SetStage(ctx, f.other, mine, directapply.StageRejected); !errors.Is(err, directapply.ErrNotFound) {
		t.Errorf("another employer's SetStage err = %v, want ErrNotFound", err)
	}
}

func TestInboxRepository_HidesAnUndeliveredApplication(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "draft", false)

	apps, err := f.repo.List(ctx, f.employer, directapply.InboxFilter{Limit: 50})
	if err != nil || len(apps) != 0 {
		t.Fatalf("List = %+v, %v; want an unsent application hidden", apps, err)
	}
	counts, _, err := f.repo.Counts(ctx, f.employer, directapply.InboxFilter{})
	if err != nil || len(counts) != 0 {
		t.Errorf("Counts = %v, %v; want nothing counted", counts, err)
	}
	if _, err := f.repo.SetStage(ctx, f.employer, id, directapply.StageReviewing); !errors.Is(err, directapply.ErrNotFound) {
		t.Errorf("SetStage on an unsent application err = %v, want ErrNotFound", err)
	}
}

func TestInboxRepository_KeepsTheCVOfTheAttemptThatWasSent(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	// A failed delivery leaves the row unsent; the retry replaces its content and CV.
	id := f.apply(t, f.vacancy, f.candidate, "first draft", false)
	if err := f.repo.Release(ctx, id); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if again := f.apply(t, f.vacancy, f.candidate, "the one sent", true); again != id {
		t.Fatalf("retry id = %d, want the same row %d", again, id)
	}
	cv, err := f.repo.CV(ctx, f.employer, id)
	if err != nil {
		t.Fatalf("CV: %v", err)
	}
	if string(cv.Content) != "the one sent" || cv.Filename != "cv.txt" || cv.ContentType != "text/plain; charset=utf-8" {
		t.Errorf("CV = %q %q %q, want the file the delivered attempt mailed", cv.Content, cv.Filename, cv.ContentType)
	}
}

func TestInboxRepository_MovesAndCountsStages(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "cv", true)

	mv, err := f.repo.SetStage(ctx, f.employer, id, directapply.StageInterview)
	if err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	if !mv.Changed || mv.CandidateID != f.candidate || mv.CandidateEmail != "c@example.test" || mv.JobID != f.vacancy ||
		mv.JobSlug != "acme-platform" || mv.Company != "Acme" {
		t.Errorf("move = %+v", mv)
	}
	if mv, err := f.repo.SetStage(ctx, f.employer, id, directapply.StageInterview); err != nil || mv.Changed {
		t.Errorf("repeated SetStage = %+v, %v; want unchanged", mv, err)
	}
	a, err := f.repo.Get(ctx, f.employer, id)
	if err != nil || a.Stage != directapply.StageInterview || a.StageChangedAt == nil {
		t.Errorf("Get = %+v, %v; want interview with a change time", a, err)
	}

	counts, _, err := f.repo.Counts(ctx, f.employer, directapply.InboxFilter{JobSlug: "acme-platform"})
	if err != nil || counts[directapply.StageInterview] != 1 || len(counts) != 1 {
		t.Errorf("Counts = %v, %v", counts, err)
	}
	if counts, _, err := f.repo.Counts(ctx, f.employer, directapply.InboxFilter{JobSlug: "another-vacancy"}); err != nil || len(counts) != 0 {
		t.Errorf("Counts for another vacancy = %v, %v; want none", counts, err)
	}
	if apps, err := f.repo.List(ctx, f.employer, directapply.InboxFilter{Stage: directapply.StageNew, Limit: 50}); err != nil || len(apps) != 0 {
		t.Errorf("List(new) = %+v, %v; want the moved application filtered out", apps, err)
	}
	if apps, err := f.repo.List(ctx, f.employer, directapply.InboxFilter{JobSlug: "acme-platform", Stage: directapply.StageInterview, Limit: 50}); err != nil || len(apps) != 1 {
		t.Errorf("List(acme-platform, interview) = %+v, %v; want it", apps, err)
	}
}

func TestInboxRepository_ATeammateSeesTheTeamsApplications(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "cv", true)
	var mate int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('dev@acme.test') RETURNING id`).Scan(&mate); err != nil {
		t.Fatalf("seed teammate: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO company_account_members (user_id, company_slug, work_email) VALUES ($1, 'acme', 'dev@acme.test')`, mate); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if apps, err := f.repo.List(ctx, mate, directapply.InboxFilter{Limit: 50}); err != nil || len(apps) != 1 || apps[0].ID != id {
		t.Fatalf("teammate's List = %+v, %v; want the application to the holder's vacancy", apps, err)
	}
	if _, err := f.repo.SetStage(ctx, mate, id, directapply.StageReviewing); err != nil {
		t.Errorf("teammate's SetStage: %v", err)
	}
}

func TestInboxRepository_ApplicationsGoWithTheCandidate(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "cv", true)
	if _, err := f.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.candidate); err != nil {
		t.Fatalf("delete candidate: %v", err)
	}
	var left int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM direct_application_cvs WHERE application_id = $1`, id).Scan(&left); err != nil || left != 0 {
		t.Errorf("kept CVs after account deletion = %d, %v; want none", left, err)
	}
}

func TestApplicationMoves_SerialiseOneApplication(t *testing.T) {
	f := newInboxFixture(t)
	moves := directapply.NewApplicationMoves(db.New(f.pool), f.pool)
	ctx := context.Background()

	firstIn, release := make(chan struct{}), make(chan struct{})
	var released bool
	releaseOnce := func() {
		if !released {
			released = true
			close(release)
		}
	}
	t.Cleanup(releaseOnce) // a failed assertion must not leave the first move blocked
	done := make(chan error, 1)
	go func() {
		done <- moves.RunMove(ctx, 42, func(context.Context, directapply.MoveScope) (func(context.Context), error) {
			close(firstIn)
			<-release
			return nil, nil
		})
	}()
	select {
	case <-firstIn:
	case err := <-done:
		t.Fatalf("the first move ended before running its callback: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the first move never ran its callback")
	}

	secondRan := make(chan struct{})
	go func() {
		_ = moves.RunMove(ctx, 42, func(context.Context, directapply.MoveScope) (func(context.Context), error) {
			close(secondRan)
			return nil, nil
		})
	}()
	select {
	case <-secondRan:
		t.Fatal("a second move of the same application ran while the first held its lock")
	case <-time.After(300 * time.Millisecond):
	}
	// Another application is not held up.
	if err := moves.RunMove(ctx, 43, func(context.Context, directapply.MoveScope) (func(context.Context), error) { return nil, nil }); err != nil {
		t.Fatalf("RunMove(43): %v", err)
	}
	releaseOnce()
	if err := <-done; err != nil {
		t.Fatalf("first RunMove: %v", err)
	}
	select {
	case <-secondRan:
	case <-time.After(5 * time.Second):
		t.Fatal("the second move never ran after the first released the lock")
	}
}

func TestApplicationMoves_RunTheMoveOnItsOwnTransaction(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "cv", true)
	moves := directapply.NewApplicationMoves(db.New(f.pool), f.pool)

	// A failed run rolls its stage write back.
	failed := errors.New("boom")
	if err := moves.RunMove(ctx, id, func(ctx context.Context, sc directapply.MoveScope) (func(context.Context), error) {
		if _, err := sc.Repo.SetStage(ctx, f.employer, id, directapply.StageRejected); err != nil {
			return nil, err
		}
		return nil, failed
	}); !errors.Is(err, failed) {
		t.Fatalf("RunMove err = %v, want the callback's", err)
	}
	if a, err := f.repo.Get(ctx, f.employer, id); err != nil || a.Stage != directapply.StageNew {
		t.Fatalf("stage after a failed run = %+v, %v; want new", a.Stage, err)
	}
	// A successful one commits it BEFORE its after-commit step, which sees the stage saved —
	// and a context that ends during that step (a slow letter) undoes nothing.
	runCtx, cancel := context.WithCancel(ctx)
	var seen directapply.Stage
	if err := moves.RunMove(runCtx, id, func(ctx context.Context, sc directapply.MoveScope) (func(context.Context), error) {
		if _, err := sc.Repo.SetStage(ctx, f.employer, id, directapply.StageInterview); err != nil {
			return nil, err
		}
		return func(context.Context) {
			a, _ := f.repo.Get(ctx, f.employer, id) // a fresh connection: only committed data
			seen = a.Stage
			cancel()
		}, nil
	}); err != nil {
		t.Fatalf("RunMove: %v", err)
	}
	if seen != directapply.StageInterview {
		t.Fatalf("the after-commit step saw %q; want the committed interview", seen)
	}
	// The lock was released despite the cancelled context: the next move runs.
	if err := moves.RunMove(ctx, id, func(context.Context, directapply.MoveScope) (func(context.Context), error) { return nil, nil }); err != nil {
		t.Fatalf("a move after a cancelled one: %v", err)
	}
	if a, err := f.repo.Get(ctx, f.employer, id); err != nil || a.Stage != directapply.StageInterview {
		t.Fatalf("stage after a successful run = %+v, %v; want interview", a.Stage, err)
	}
}

func TestInboxRepository_PreservesStageWhenUpdatingWorkspace(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	id := f.apply(t, f.vacancy, f.candidate, "cv", true)
	if _, err := f.repo.SetStage(ctx, f.employer, id, directapply.StageInterview); err != nil {
		t.Fatalf("SetStage: %v", err)
	}
	notes := "Discuss scheduling"
	if err := f.repo.UpdateWorkspace(ctx, f.other, id, true, &notes); !errors.Is(err, directapply.ErrNotFound) {
		t.Fatalf("other employer updated workspace: %v", err)
	}
	if err := f.repo.UpdateWorkspace(ctx, f.employer, id, true, &notes); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	app, err := f.repo.Get(ctx, f.employer, id)
	if err != nil || app.ViewedAt == nil || app.EmployerNotes != notes || app.Stage != directapply.StageInterview {
		t.Fatalf("workspace = %+v, %v; want viewed and notes saved with the original stage", app, err)
	}
}
